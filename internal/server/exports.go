package server

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

// ---- your deals, out and in ----

// exportDoc is one person's pipeline and deals with their timelines, as JSON.
type exportDoc struct {
	App      string         `json:"app"`
	Version  string         `json:"version"`
	Exported string         `json:"exported_at"`
	Stages   []*store.Stage `json:"stages"`
	Deals    []exportDeal   `json:"deals"`
}

type exportDeal struct {
	store.Deal
	Key        string            `json:"key"`
	Timeline   []*store.Activity `json:"timeline"`
	Elsewhere  []*store.Made     `json:"elsewhere"`
	StageLabel string            `json:"stage"`
}

func (s *Server) export(ctx context.Context, uid int64) exportDoc {
	doc := exportDoc{App: "pocket-crm", Version: Version, Exported: store.Now(), Deals: []exportDeal{}}
	doc.Stages, _ = s.store.ListStages(ctx, uid)
	all, _ := s.store.AllDeals(ctx, uid)
	for _, d := range all {
		key := firstNonEmpty(d.ClientKey, fmt.Sprintf("crm:%d", d.ID))
		acts, _ := s.store.ListActivities(ctx, uid, d.ID, 1000)
		made, _ := s.store.ListMade(ctx, uid, d.ID)
		doc.Deals = append(doc.Deals, exportDeal{Deal: *d, Key: key, Timeline: acts, Elsewhere: made, StageLabel: d.StageName})
	}
	return doc
}

func (s *Server) handleExportJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-crm-export.json"`)
	writeJSON(w, http.StatusOK, s.export(r.Context(), actorID(r.Context())))
}

var csvHead = []string{"Title", "Stage", "Status", "Value", "Currency", "Chance", "Expected close", "Source", "Contact", "Email", "Phone", "Company",
	"Next step", "Next step due", "Tags", "Lost reason", "Description", "Added", "Closed", "Last activity"}

// handleExportCSV is every deal in one table (a spreadsheet opens it).
func (s *Server) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	all, err := s.store.AllDeals(ctx, actorID(ctx))
	if err != nil {
		s.fail(w, err, "export")
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="pocket-crm-deals.csv"`)
	_, _ = w.Write([]byte("\ufeff"))
	cw := csv.NewWriter(w)
	_ = cw.Write(csvHead)
	for _, d := range all {
		_ = cw.Write([]string{d.Title, d.StageName, d.Status, strconv.FormatFloat(d.Amount, 'f', 2, 64), d.Currency, strconv.Itoa(d.Chance),
			d.ExpectedClose, d.Source, d.ContactName, d.ContactEmail, d.ContactPhone, d.Company, d.NextAction, d.NextDue,
			strings.Join(d.Tags, ", "), d.LostReason, d.Description, day10(d.CreatedAt), day10(d.ClosedAt), day10(d.LastActivityAt)})
	}
	cw.Flush()
}

// handleImport takes a Pocket CRM export (again and again: nothing is
// doubled), or deals as CSV from Pipedrive, HubSpot, Zoho, a spreadsheet.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	var data []byte
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBodyBytes)
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		f, _, err := r.FormFile("file")
		if err != nil {
			writeErr(w, http.StatusBadRequest, "no file")
			return
		}
		defer f.Close()
		if data, err = io.ReadAll(f); err != nil {
			writeErr(w, http.StatusBadRequest, "could not read the file")
			return
		}
	} else {
		b, err := s.readBody(r)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "the file is too big")
			return
		}
		data = b
	}
	data = bytes.TrimPrefix(data, []byte("\ufeff"))
	if bytes.HasPrefix(bytes.TrimSpace(data), []byte("{")) {
		var doc exportDoc
		if err := json.Unmarshal(data, &doc); err != nil || doc.App != "pocket-crm" {
			writeErr(w, http.StatusBadRequest, "that is not a Pocket CRM export")
			return
		}
		writeJSON(w, http.StatusOK, s.importPocket(ctx, uid, doc))
		return
	}
	out, err := s.importCSV(ctx, uid, data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// stageFor finds (or adds) the stage a deal goes to on import.
func (s *Server) stageFor(ctx context.Context, uid int64, name, status string) int64 {
	name = strings.TrimSpace(name)
	if st, err := s.store.StageByName(ctx, uid, name); err == nil && name != "" && (status == "" || strings.EqualFold(st.Kind, status) || st.Kind != store.StatusOpen) {
		return st.ID
	}
	switch strings.ToLower(status) {
	case store.StatusWon, "closed won", "closed-won":
		if st, err := s.store.FirstStage(ctx, uid, store.StatusWon); err == nil {
			return st.ID
		}
	case store.StatusLost, "closed lost", "closed-lost":
		if st, err := s.store.FirstStage(ctx, uid, store.StatusLost); err == nil {
			return st.ID
		}
	}
	if name != "" {
		low := strings.ToLower(name)
		kind := store.StatusOpen
		switch {
		case strings.Contains(low, "won"):
			kind = store.StatusWon
		case strings.Contains(low, "lost"):
			kind = store.StatusLost
		}
		if kind != store.StatusOpen {
			if st, err := s.store.FirstStage(ctx, uid, kind); err == nil {
				return st.ID
			}
		}
		st := &store.Stage{UserID: uid, Name: clip(name, 60), Kind: store.StatusOpen, Probability: 50}
		if err := s.store.CreateStage(ctx, st); err == nil {
			return st.ID
		}
	}
	st, _ := s.store.FirstStage(ctx, uid, store.StatusOpen)
	if st == nil {
		return 0
	}
	return st.ID
}

func (s *Server) importPocket(ctx context.Context, uid int64, doc exportDoc) map[string]any {
	added, skipped := 0, 0
	for _, ed := range doc.Deals {
		key := firstNonEmpty(ed.ClientKey, ed.Key)
		if key == "" {
			key = fmt.Sprintf("crm:%d", ed.ID)
		}
		if _, err := s.store.DealByClientKey(ctx, uid, key); err == nil {
			skipped++
			continue
		}
		d := ed.Deal
		d.ID, d.UserID, d.ClientKey, d.SourceRef = 0, uid, key, ""
		d.StageID = s.stageFor(ctx, uid, ed.StageLabel, d.Status)
		if err := s.store.CreateDeal(ctx, &d, nil); err != nil {
			skipped++
			continue
		}
		// CreateDeal takes the stage's status: closed deals keep their day
		if d.Status != store.StatusOpen && ed.ClosedAt != "" {
			_, _ = s.store.DB.ExecContext(ctx, `UPDATE deals SET closed_at = ? WHERE user_id = ? AND id = ?`, ed.ClosedAt, uid, d.ID)
		}
		for i := len(ed.Timeline) - 1; i >= 0; i-- {
			a := *ed.Timeline[i]
			a.ID, a.UserID, a.DealID = 0, uid, d.ID
			_ = s.store.AddActivity(ctx, &a)
		}
		_, _ = s.store.DB.ExecContext(ctx, `UPDATE deals SET last_activity_at = ? WHERE user_id = ? AND id = ?`, firstNonEmpty(ed.LastActivityAt, d.CreatedAt), uid, d.ID)
		for _, m := range ed.Elsewhere {
			if m.Error != "" {
				continue
			}
			mm := *m
			mm.ID, mm.DealID = 0, d.ID
			_ = s.store.AddMade(ctx, uid, &mm)
		}
		added++
	}
	return map[string]any{"deals": added, "skipped": skipped}
}

// columns other tools name their deal fields (lower case).
var csvColumns = map[string][]string{
	"title":    {"title", "deal", "deal name", "deal title", "name", "opportunity", "opportunity name", "naziv", "posao"},
	"stage":    {"stage", "deal stage", "pipeline stage", "faza", "stage name"},
	"status":   {"status", "deal status", "state"},
	"value":    {"value", "amount", "deal value", "deal amount", "expected revenue", "iznos", "vrijednost"},
	"currency": {"currency", "deal currency", "valuta"},
	"chance":   {"chance", "probability", "probability (%)", "deal probability", "win probability"},
	"close":    {"expected close", "expected close date", "close date", "closing date", "datum zatvaranja"},
	"source":   {"source", "lead source", "deal source", "izvor"},
	"contact":  {"contact", "contact person", "person", "contact name", "kontakt", "associated contact"},
	"email":    {"email", "e-mail", "contact email", "person - email", "email address"},
	"phone":    {"phone", "contact phone", "person - phone", "telephone", "telefon"},
	"company":  {"company", "organization", "organisation", "account", "account name", "company name", "associated company", "firma", "kompanija"},
	"step":     {"next step", "next activity", "next action", "sljedeći korak"},
	"due":      {"next step due", "next activity date", "next action date"},
	"tags":     {"tags", "labels", "label"},
	"lost":     {"lost reason", "closed lost reason", "reason lost"},
	"desc":     {"description", "notes", "note", "details", "opis", "bilješke"},
	"added":    {"added", "created", "create date", "deal created", "created at", "date added", "add time"},
	"closed":   {"closed", "won time", "close time", "closed date", "closed at"},
	"id":       {"id", "deal id", "record id"},
}

// csvDate reads the days other tools write (2026-03-04, 04.03.2026, 3/4/2026, with a time or not).
func csvDate(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	if len(v) >= 10 {
		if _, err := time.Parse("2006-01-02", v[:10]); err == nil {
			return v[:10]
		}
	}
	for _, f := range []string{"02.01.2006", "2.1.2006", "02/01/2006", "1/2/2006", "01/02/2006", "Jan 2, 2006", "2 Jan 2006"} {
		part := v
		if i := strings.IndexAny(v, " T"); i > 0 && !strings.Contains(f, " ") {
			part = v[:i]
		}
		if t, err := time.Parse(f, part); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return ""
}

func (s *Server) importCSV(ctx context.Context, uid int64, data []byte) (map[string]any, error) {
	rd := csv.NewReader(bytes.NewReader(data))
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	head := data[:min(len(data), 4096)]
	if bytes.Count(head, []byte(";")) > bytes.Count(head, []byte(",")) {
		rd.Comma = ';'
	} else if bytes.Count(head, []byte("\t")) > bytes.Count(head, []byte(",")) {
		rd.Comma = '\t'
	}
	recs, err := rd.ReadAll()
	if err != nil || len(recs) < 2 {
		return nil, errors.New("this does not look like deals in a CSV file (a header row and at least one deal)")
	}
	idx := map[string]int{}
	for i, h := range recs[0] {
		lh := strings.ToLower(strings.TrimSpace(strings.Trim(h, "\ufeff\"")))
		for k, names := range csvColumns {
			if _, ok := idx[k]; !ok && oneOf(lh, names) {
				idx[k] = i
			}
		}
	}
	if _, ok := idx["title"]; !ok {
		if _, ok := idx["company"]; !ok {
			return nil, errors.New("the first row needs a column for the deal's name (Title, Deal, Name) or the company")
		}
	}
	st := s.settingsOf(ctx, uid)
	added, skipped := 0, 0
	for _, rec := range recs[1:] {
		get := func(k string) string {
			if i, ok := idx[k]; ok && i < len(rec) {
				return strings.TrimSpace(rec[i])
			}
			return ""
		}
		title := firstNonEmpty(get("title"), get("company"), get("contact"))
		if title == "" {
			continue
		}
		sum := sha1.Sum([]byte(strings.Join(rec, "\x1f")))
		key := "csv:" + hex.EncodeToString(sum[:])[:16]
		if id := get("id"); id != "" {
			key = "csv:id:" + id
		}
		if _, err := s.store.DealByClientKey(ctx, uid, key); err == nil {
			skipped++
			continue
		}
		d := &store.Deal{UserID: uid, Title: clip(title, 200), Currency: st.Currency, Probability: -1, Tags: []string{}, ClientKey: key,
			ContactName: clip(get("contact"), 200), ContactEmail: strings.ToLower(clip(get("email"), 200)), ContactPhone: clip(get("phone"), 60),
			Company: clip(get("company"), 200), NextAction: clip(get("step"), 300), NextDue: csvDate(get("due")), ExpectedClose: csvDate(get("close")),
			LostReason: clip(get("lost"), 200), Description: clip(get("desc"), 20000)}
		if d.ContactEmail != "" && !reEmail.MatchString(d.ContactEmail) {
			d.ContactEmail = ""
		}
		if v := parseAmount(get("value")); v > 0 {
			d.Value = int64(v*100 + 0.5)
		}
		if c := strings.ToUpper(get("currency")); reCur.MatchString(c) {
			d.Currency = c
		}
		if p := parseAmount(strings.TrimSuffix(get("chance"), "%")); p > 0 && p <= 100 {
			d.Probability = int(p)
		}
		if src := strings.ToLower(get("source")); src != "" {
			d.Source = "other"
			for _, known := range store.Sources {
				if strings.Contains(src, known) {
					d.Source = known
				}
			}
		}
		if t := get("tags"); t != "" {
			d.Tags = strings.FieldsFunc(t, func(r rune) bool { return r == ',' || r == ';' || r == '|' })
		}
		if a := csvDate(get("added")); a != "" {
			d.CreatedAt = a + "T09:00:00Z"
		}
		d.StageID = s.stageFor(ctx, uid, get("stage"), get("status"))
		if err := s.store.CreateDeal(ctx, d, &store.Activity{Kind: "created", Title: "Brought in from a CSV file"}); err != nil {
			skipped++
			continue
		}
		if c := csvDate(get("closed")); c != "" && d.Status != store.StatusOpen {
			_, _ = s.store.DB.ExecContext(ctx, `UPDATE deals SET closed_at = ? WHERE user_id = ? AND id = ?`, c+"T12:00:00Z", uid, d.ID)
		}
		added++
	}
	return map[string]any{"deals": added, "skipped": skipped}, nil
}
