package server

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

// ---- deals, the pipeline and the timeline ----

var (
	reHex   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	reCur   = regexp.MustCompile(`^[A-Z]{3}$`)
	reEmail = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	reTime  = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// userErr is a reason the person can act on (400, not 502).
type userErr struct{ msg string }

func (e *userErr) Error() string { return e.msg }

// today is the local day for the person (the box's zone).
func today() string { return time.Now().Format("2006-01-02") }

// decorate fills what lists and pages show besides the stored fields.
func decorate(st store.Settings, ds ...*store.Deal) {
	t := today()
	staleBefore := time.Now().AddDate(0, 0, -max(st.StaleDays, 1)).UTC().Format(time.RFC3339Nano)
	for _, d := range ds {
		d.ClientKey = ""
		if d.Status != store.StatusOpen {
			continue
		}
		d.Overdue = d.NextDue != "" && d.NextDue < t
		d.Stale = st.StaleDays > 0 && d.LastActivityAt < staleBefore && d.NextDue == ""
		if at, err := time.Parse(time.RFC3339Nano, d.StageChangedAt); err == nil {
			d.DaysInStage = int(time.Since(at).Hours() / 24)
		}
	}
}

func (s *Server) settingsOf(ctx context.Context, uid int64) store.Settings {
	st, _ := s.store.GetSettings(ctx, uid)
	return st
}

// ---- stages ----

func (s *Server) handleListStages(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListStages(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "stages")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type stageIn struct {
	Name        *string `json:"name"`
	Kind        *string `json:"kind"`
	Probability *int    `json:"probability"`
	Color       *string `json:"color"`
	Position    int     `json:"position"`
}

func (in stageIn) apply(st *store.Stage) error {
	if in.Name != nil {
		st.Name = clip(strings.TrimSpace(*in.Name), 60)
	}
	if st.Name == "" {
		return &userErr{"give the stage a name"}
	}
	if in.Kind != nil {
		if !oneOf(*in.Kind, []string{store.StatusOpen, store.StatusWon, store.StatusLost}) {
			return &userErr{"a stage is open, won or lost"}
		}
		st.Kind = *in.Kind
	}
	if in.Probability != nil {
		if *in.Probability < 0 || *in.Probability > 100 {
			return &userErr{"the chance is 0 to 100"}
		}
		st.Probability = *in.Probability
	}
	switch st.Kind {
	case store.StatusWon:
		st.Probability = 100
	case store.StatusLost:
		st.Probability = 0
	}
	if in.Color != nil {
		if *in.Color != "" && !reHex.MatchString(*in.Color) {
			return &userErr{"the colour is like #1c7ed6"}
		}
		st.Color = *in.Color
	}
	return nil
}

func (s *Server) handleCreateStage(w http.ResponseWriter, r *http.Request) {
	var in stageIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	st := &store.Stage{UserID: actorID(r.Context()), Kind: store.StatusOpen, Probability: 50, Position: in.Position}
	if err := in.apply(st); err != nil {
		s.userFail(w, err, "stage")
		return
	}
	if err := s.store.CreateStage(r.Context(), st); err != nil {
		s.fail(w, err, "create stage")
		return
	}
	writeJSON(w, http.StatusCreated, st)
}

func (s *Server) handleUpdateStage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, err := s.store.GetStage(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "stage")
		return
	}
	var in stageIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.apply(st); err != nil {
		s.userFail(w, err, "stage")
		return
	}
	if err := s.store.UpdateStage(ctx, st); err != nil {
		s.fail(w, err, "save stage")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleDeleteStage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	err := s.store.DeleteStage(ctx, actorID(ctx), idParam(r, "id"), int64(queryInt(r, "move_to")))
	if errors.Is(err, store.ErrStageInUse) {
		writeErr(w, http.StatusConflict, "deals are still in this stage: pick where they go")
		return
	}
	if err != nil {
		s.fail(w, err, "delete stage")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleOrderStages(w http.ResponseWriter, r *http.Request) {
	var in struct {
		IDs []int64 `json:"ids"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	ctx := r.Context()
	if err := s.store.ReorderStages(ctx, actorID(ctx), in.IDs); err != nil {
		s.fail(w, err, "order stages")
		return
	}
	s.handleListStages(w, r)
}

// ---- deals ----

func (s *Server) handleListDeals(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := r.URL.Query()
	f := store.DealFilter{Status: q.Get("status"), StageID: int64(queryInt(r, "stage")), Search: q.Get("q"), Company: q.Get("company"),
		Email: q.Get("email"), Due: q.Get("due"), Starred: queryBool(r, "starred"), Tag: q.Get("tag"), Source: q.Get("source"),
		Sort: q.Get("sort"), Limit: queryInt(r, "limit"), Offset: queryInt(r, "offset")}
	list, total, err := s.store.ListDeals(ctx, uid, f)
	if err != nil {
		s.fail(w, err, "deals")
		return
	}
	decorate(s.settingsOf(ctx, uid), list...)
	writeJSON(w, http.StatusOK, map[string]any{"items": list, "total": total})
}

// dealIn is a deal as sent: every field optional (PUT changes what is sent).
type dealIn struct {
	Title         *string   `json:"title"`
	StageID       *int64    `json:"stage_id"`
	Stage         *string   `json:"stage"` // by name (links, imports)
	Value         *int64    `json:"value"`
	Amount        *float64  `json:"amount"`
	Currency      *string   `json:"currency"`
	Probability   *int      `json:"probability"`
	ExpectedClose *string   `json:"expected_close"`
	Source        *string   `json:"source"`
	ContactName   *string   `json:"contact_name"`
	ContactEmail  *string   `json:"contact_email"`
	ContactPhone  *string   `json:"contact_phone"`
	ContactRef    *string   `json:"contact_ref"`
	Company       *string   `json:"company"`
	CompanyRef    *string   `json:"company_ref"`
	Description   *string   `json:"description"`
	NextAction    *string   `json:"next_action"`
	NextDue       *string   `json:"next_due"`
	Tags          *[]string `json:"tags"`
	Starred       *bool     `json:"starred"`
	LostReason    *string   `json:"lost_reason"`
	OutcomeNote   *string   `json:"outcome_note"`
	SourceRef     string    `json:"source_ref"` // links: what it came from, so it is never added twice
	Note          string    `json:"note"`       // a first line for the timeline
}

func (in dealIn) apply(d *store.Deal) error {
	txt := func(p *string, dst *string, n int) {
		if p != nil {
			*dst = clip(strings.TrimSpace(*p), n)
		}
	}
	txt(in.Title, &d.Title, 200)
	txt(in.ContactName, &d.ContactName, 200)
	txt(in.ContactEmail, &d.ContactEmail, 200)
	txt(in.ContactPhone, &d.ContactPhone, 60)
	txt(in.ContactRef, &d.ContactRef, 60)
	txt(in.Company, &d.Company, 200)
	txt(in.CompanyRef, &d.CompanyRef, 60)
	txt(in.Description, &d.Description, 20000)
	txt(in.NextAction, &d.NextAction, 300)
	txt(in.NextDue, &d.NextDue, 10)
	txt(in.ExpectedClose, &d.ExpectedClose, 10)
	txt(in.LostReason, &d.LostReason, 200)
	txt(in.OutcomeNote, &d.OutcomeNote, 5000)
	d.ContactEmail = strings.ToLower(d.ContactEmail)
	if d.ContactEmail != "" && !reEmail.MatchString(d.ContactEmail) {
		return &userErr{"that email address does not look right"}
	}
	if !validDate(d.NextDue) || !validDate(d.ExpectedClose) {
		return &userErr{"days are YYYY-MM-DD"}
	}
	if in.Value != nil {
		d.Value = *in.Value
	}
	if in.Amount != nil {
		if math.IsNaN(*in.Amount) || math.IsInf(*in.Amount, 0) {
			return &userErr{"the value is a number"}
		}
		d.Value = int64(math.Round(*in.Amount * 100))
	}
	if d.Value < 0 || d.Value > 1e15 {
		return &userErr{"the value is 0 or more"}
	}
	if in.Currency != nil {
		c := strings.ToUpper(strings.TrimSpace(*in.Currency))
		if c == "KM" {
			c = "BAM"
		}
		if !reCur.MatchString(c) {
			return &userErr{"the currency is a three-letter code like EUR"}
		}
		d.Currency = c
	}
	if in.Probability != nil {
		if *in.Probability < -1 || *in.Probability > 100 {
			return &userErr{"the chance is 0 to 100 (or -1 for the stage's)"}
		}
		d.Probability = *in.Probability
	}
	if in.Source != nil {
		src := strings.TrimSpace(strings.ToLower(*in.Source))
		if src != "" && !oneOf(src, store.Sources) {
			src = "other"
		}
		d.Source = src
	}
	if in.Tags != nil {
		d.Tags = *in.Tags
	}
	if in.Starred != nil {
		d.Starred = *in.Starred
	}
	if d.Title == "" {
		d.Title = firstNonEmpty(d.Company, d.ContactName, d.ContactEmail)
	}
	if d.Title == "" {
		return &userErr{"give the deal a name (or say who it is with)"}
	}
	return nil
}

func (s *Server) userFail(w http.ResponseWriter, err error, what string) {
	var ue *userErr
	if errors.As(err, &ue) {
		writeErr(w, http.StatusBadRequest, ue.msg)
		return
	}
	s.fail(w, err, what)
}

// resolveStage turns stage_id / stage (a name) into one of the person's stages.
func (s *Server) resolveStage(ctx context.Context, uid int64, in dealIn) (*store.Stage, error) {
	switch {
	case in.StageID != nil && *in.StageID > 0:
		st, err := s.store.GetStage(ctx, uid, *in.StageID)
		if err != nil {
			return nil, &userErr{"there is no such stage"}
		}
		return st, nil
	case in.Stage != nil && strings.TrimSpace(*in.Stage) != "":
		name := strings.TrimSpace(*in.Stage)
		if st, err := s.store.StageByName(ctx, uid, name); err == nil {
			return st, nil
		}
		switch strings.ToLower(name) {
		case "won", "lost", "open":
			return s.store.FirstStage(ctx, uid, strings.ToLower(name))
		}
		return nil, &userErr{"there is no stage called " + name}
	}
	return nil, nil
}

func (s *Server) handleCreateDeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in dealIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	app := linkApp(ctx)
	ref := clip(strings.TrimSpace(in.SourceRef), 200)
	if ref != "" {
		if app != "" && !strings.HasPrefix(ref, app+":") {
			ref = app + ":" + ref
		}
		if d, err := s.store.DealBySource(ctx, uid, ref); err == nil {
			decorate(s.settingsOf(ctx, uid), d)
			writeJSON(w, http.StatusOK, d)
			return
		}
	}
	d, err := s.createDeal(ctx, uid, in, app, ref)
	if err != nil {
		s.userFail(w, err, "create deal")
		return
	}
	writeJSON(w, http.StatusCreated, d)
}

// createDeal stores a new deal with its first timeline line and tells the box.
func (s *Server) createDeal(ctx context.Context, uid int64, in dealIn, app, ref string) (*store.Deal, error) {
	st := s.settingsOf(ctx, uid)
	d := &store.Deal{UserID: uid, Currency: st.Currency, Probability: -1, SourceApp: app, SourceRef: ref, Tags: []string{}}
	if err := in.apply(d); err != nil {
		return nil, err
	}
	if stage, err := s.resolveStage(ctx, uid, in); err != nil {
		return nil, err
	} else if stage != nil {
		d.StageID = stage.ID
	}
	first := &store.Activity{Kind: "created", Title: "Deal added", Body: strings.TrimSpace(in.Note), SourceApp: app}
	if a, ok := appBySlug(app); ok {
		first.Title, first.App = "Added from "+a.name, app
	}
	if err := s.store.CreateDeal(ctx, d, first); err != nil {
		return nil, err
	}
	d, err := s.store.GetDeal(ctx, uid, d.ID)
	if err != nil {
		return nil, err
	}
	decorate(st, d)
	s.publish(ctx, uid, "crm.deal_created", fmt.Sprintf("deal:%d", d.ID), s.dealEvent(d))
	if d.NextAction != "" && st.TaskMirror {
		go s.mirrorStep(context.WithoutCancel(ctx), uid, d.ID)
	}
	return d, nil
}

// dealEvent is a deal as events carry it.
func (s *Server) dealEvent(d *store.Deal) map[string]any {
	return map[string]any{"deal_id": d.ID, "title": d.Title, "stage": d.StageName, "status": d.Status, "value": d.Value, "amount": d.Amount,
		"currency": d.Currency, "contact_name": d.ContactName, "contact_email": d.ContactEmail, "company": d.Company,
		"next_action": d.NextAction, "next_due": d.NextDue, "url": s.dealLink(d.ID)}
}

// dealLink is the owner's page of a deal.
func (s *Server) dealLink(id int64) string {
	return strings.TrimRight(s.cfg.BaseURL, "/") + fmt.Sprintf("/deals/%d", id)
}

func (s *Server) handleGetDeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDeal(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "deal")
		return
	}
	decorate(s.settingsOf(ctx, uid), d)
	acts, _ := s.store.ListActivities(ctx, uid, d.ID, 300)
	made, _ := s.store.ListMade(ctx, uid, d.ID)
	writeJSON(w, http.StatusOK, map[string]any{"deal": d, "activities": acts, "made": made, "url": s.dealLink(d.ID)})
}

func (s *Server) handleUpdateDeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDeal(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "deal")
		return
	}
	if app := linkApp(ctx); app != "" && d.SourceApp != app {
		writeErr(w, http.StatusForbidden, "this deal was not added by "+app)
		return
	}
	var in dealIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	oldStep, oldDue := d.NextAction, d.NextDue
	if err := in.apply(d); err != nil {
		s.userFail(w, err, "deal")
		return
	}
	if d.NextAction == "" {
		d.NextDue = ""
	}
	if err := s.store.UpdateDeal(ctx, d); err != nil {
		s.fail(w, err, "save deal")
		return
	}
	if stage, err := s.resolveStage(ctx, uid, in); err != nil {
		s.userFail(w, err, "deal")
		return
	} else if stage != nil && stage.ID != d.StageID {
		if _, err := s.moveDeal(ctx, uid, d.ID, stage.ID, nil, nil, "", ""); err != nil {
			s.userFail(w, err, "move deal")
			return
		}
	}
	if d.NextAction != oldStep || d.NextDue != oldDue {
		go s.mirrorStep(context.WithoutCancel(ctx), uid, d.ID)
	}
	d, _ = s.store.GetDeal(ctx, uid, d.ID)
	decorate(s.settingsOf(ctx, uid), d)
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) handleDeleteDeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteDeal(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "delete deal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// moveDeal puts a deal in a stage and writes it on the timeline; winning
// and losing are told to the box.
func (s *Server) moveDeal(ctx context.Context, uid, id, stageID int64, before, after *float64, reason, note string) (*store.Deal, error) {
	prev, err := s.store.GetDeal(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	st, err := s.store.GetStage(ctx, uid, stageID)
	if err != nil {
		return nil, &userErr{"there is no such stage"}
	}
	old, err := s.store.MoveDeal(ctx, uid, id, stageID, before, after)
	if err != nil {
		return nil, err
	}
	d, err := s.store.GetDeal(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	if old == nil || old.ID == st.ID {
		return d, nil
	}
	if st.Kind == store.StatusLost && (reason != "" || note != "") {
		d.LostReason, d.OutcomeNote = clip(reason, 200), clip(note, 5000)
		_ = s.store.UpdateDeal(ctx, d)
	} else if st.Kind == store.StatusWon && note != "" {
		d.OutcomeNote = clip(note, 5000)
		_ = s.store.UpdateDeal(ctx, d)
	}
	a := &store.Activity{UserID: uid, DealID: id, Kind: "stage", Title: fmt.Sprintf("Moved from %s to %s", old.Name, st.Name), SourceApp: linkApp(ctx)}
	topic := "crm.deal_moved"
	switch {
	case st.Kind == store.StatusWon && prev.Status != store.StatusWon:
		a.Kind, a.Title, a.Body, topic = "won", "Won 🎉", note, "crm.deal_won"
	case st.Kind == store.StatusLost && prev.Status != store.StatusLost:
		a.Kind, a.Title, a.Body, topic = "lost", "Lost", joinNonEmpty(" — ", reason, note), "crm.deal_lost"
	case st.Kind == store.StatusOpen && prev.Status != store.StatusOpen:
		a.Title = "Opened again, in " + st.Name
	}
	_ = s.store.AddActivity(ctx, a)
	d, _ = s.store.GetDeal(ctx, uid, id)
	payload := s.dealEvent(d)
	payload["from_stage"] = old.Name
	s.publish(ctx, uid, topic, fmt.Sprintf("deal:%d", id), payload)
	return d, nil
}

func (s *Server) handleMoveDeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in struct {
		StageID int64    `json:"stage_id"`
		Before  *float64 `json:"before"` // the position of the deal above
		After   *float64 `json:"after"`  // and below
		Reason  string   `json:"lost_reason"`
		Note    string   `json:"note"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	d, err := s.moveDeal(ctx, uid, idParam(r, "id"), in.StageID, in.Before, in.After, strings.TrimSpace(in.Reason), strings.TrimSpace(in.Note))
	if err != nil {
		s.userFail(w, err, "move deal")
		return
	}
	decorate(s.settingsOf(ctx, uid), d)
	writeJSON(w, http.StatusOK, d)
}

// handleStepDone ticks off the next step (it goes on the timeline, and is
// ticked in Pocket Tasks too) and sets the one after it.
func (s *Server) handleStepDone(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDeal(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "deal")
		return
	}
	var in struct {
		Kind       string `json:"kind"` // what it was: step | call | meeting | email
		Note       string `json:"note"`
		NextAction string `json:"next_action"`
		NextDue    string `json:"next_due"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if !validDate(in.NextDue) {
		writeErr(w, http.StatusBadRequest, "days are YYYY-MM-DD")
		return
	}
	if !oneOf(in.Kind, store.ActivityKinds) {
		in.Kind = "step"
	}
	if d.NextAction != "" || strings.TrimSpace(in.Note) != "" {
		_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, DealID: d.ID, Kind: in.Kind, Title: firstNonEmpty(d.NextAction, "Done"), Body: clip(strings.TrimSpace(in.Note), 20000)})
	}
	oldTask := d.NextTaskRef
	next := clip(strings.TrimSpace(in.NextAction), 300)
	due := in.NextDue
	if next == "" {
		due = ""
	}
	if err := s.store.SetNextStep(ctx, uid, d.ID, next, due, ""); err != nil {
		s.fail(w, err, "next step")
		return
	}
	go func(ctx context.Context) {
		if oldTask != "" {
			s.completeTask(ctx, uid, oldTask)
		}
		if next != "" {
			s.mirrorStep(ctx, uid, d.ID)
		}
	}(context.WithoutCancel(ctx))
	d, _ = s.store.GetDeal(ctx, uid, d.ID)
	decorate(s.settingsOf(ctx, uid), d)
	writeJSON(w, http.StatusOK, d)
}

// ---- the timeline ----

func (s *Server) handleListActivities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	if _, err := s.store.GetDeal(ctx, uid, idParam(r, "id")); err != nil {
		s.fail(w, err, "deal")
		return
	}
	list, err := s.store.ListActivities(ctx, uid, idParam(r, "id"), queryInt(r, "limit"))
	if err != nil {
		s.fail(w, err, "timeline")
		return
	}
	writeJSON(w, http.StatusOK, list)
}

type activityIn struct {
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Body  string `json:"body"`
	At    string `json:"at"`
}

func (in *activityIn) clean() error {
	in.Kind = strings.TrimSpace(in.Kind)
	if in.Kind == "" {
		in.Kind = "note"
	}
	if !oneOf(in.Kind, store.ActivityKinds) {
		return &userErr{"a line is a note, call, meeting, email or step"}
	}
	in.Title, in.Body = clip(strings.TrimSpace(in.Title), 300), clip(strings.TrimSpace(in.Body), 20000)
	if in.Title == "" && in.Body == "" {
		return &userErr{"write something first"}
	}
	if in.At != "" {
		t, err := parseWhen(in.At)
		if err != nil {
			return &userErr{"the time is like 2026-11-07T14:30"}
		}
		in.At = t.UTC().Format(time.RFC3339Nano)
	}
	return nil
}

// parseWhen reads a time as people and apps send it.
func parseWhen(v string) (time.Time, error) {
	for _, f := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(f, v, time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("bad time")
}

func (s *Server) handleAddActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var in activityIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.clean(); err != nil {
		s.userFail(w, err, "timeline")
		return
	}
	a := &store.Activity{UserID: uid, DealID: idParam(r, "id"), Kind: in.Kind, Title: in.Title, Body: in.Body, At: in.At, SourceApp: linkApp(ctx)}
	if sa, ok := appBySlug(a.SourceApp); ok {
		a.App = sa.slug
	}
	if err := s.store.AddActivity(ctx, a); err != nil {
		s.fail(w, err, "timeline")
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) handleUpdateActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	a, err := s.store.GetActivity(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "timeline")
		return
	}
	if !oneOf(a.Kind, store.ActivityKinds) {
		writeErr(w, http.StatusBadRequest, "this line was written by the app")
		return
	}
	in := activityIn{Kind: a.Kind, Title: a.Title, Body: a.Body}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := in.clean(); err != nil {
		s.userFail(w, err, "timeline")
		return
	}
	a.Kind, a.Title, a.Body = in.Kind, in.Title, in.Body
	if in.At != "" {
		a.At = in.At
	}
	if err := s.store.UpdateActivity(ctx, a); err != nil {
		s.fail(w, err, "timeline")
		return
	}
	writeJSON(w, http.StatusOK, a)
}

func (s *Server) handleDeleteActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteActivity(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "timeline")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteMade(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.store.DeleteMade(ctx, actorID(ctx), idParam(r, "id")); err != nil {
		s.fail(w, err, "linked item")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	tags, err := s.store.Tags(r.Context(), actorID(r.Context()))
	if err != nil {
		s.fail(w, err, "tags")
		return
	}
	writeJSON(w, http.StatusOK, tags)
}
