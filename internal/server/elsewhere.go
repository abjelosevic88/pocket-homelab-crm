package server

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

// ---- what the other apps know about a deal's person ----
//
// The emails exchanged with them (Pocket Mail), the meetings with them
// (Pocket Calendar), the answers they sent to your forms (Pocket Forms) and
// their invoices (Pocket Invoicing), read live and never stored. Each app is
// asked on its own: one that is missing or off only greys out its own part.

type elseItem struct {
	Kind   string `json:"kind"` // email | meeting | answer | invoice
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	At     string `json:"at"`
	URL    string `json:"url,omitempty"`
	State  string `json:"state,omitempty"`
	Amount string `json:"amount,omitempty"`
	Out    bool   `json:"out,omitempty"` // an email you sent
	Ref    string `json:"ref,omitempty"`
}

type elseSection struct {
	App    string     `json:"app"`
	Name   string     `json:"name"`
	State  string     `json:"state"` // ok | off | error | none (nothing to ask with)
	Detail string     `json:"detail,omitempty"`
	Fix    string     `json:"fix,omitempty"`
	Items  []elseItem `json:"items"`
	Open   string     `json:"open_url,omitempty"`
}

func (s *Server) handleElsewhere(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.ownDeal(w, r)
	if !ok {
		return
	}
	c := s.clients(ctx, d.UserID)
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	type job struct {
		a   pocketApp
		run func(context.Context, *siblingConn) ([]elseItem, error)
		sc  string
		why string // empty = there is something to ask with
	}
	jobs := []job{
		{appMail, func(ctx context.Context, sib *siblingConn) ([]elseItem, error) { return s.mailWith(ctx, sib, d) }, scopeMailRead, whyNot(d.ContactEmail == "", "add their email to see your emails with them")},
		{appCalendar, func(ctx context.Context, sib *siblingConn) ([]elseItem, error) { return s.meetingsWith(ctx, sib, d) }, scopeCalendarRead, whyNot(d.ContactName == "" && d.ContactEmail == "" && d.Company == "", "say who the deal is with to see your meetings with them")},
		{appForms, func(ctx context.Context, sib *siblingConn) ([]elseItem, error) { return s.answersFrom(ctx, sib, d) }, scopeFormsRead, whyNot(d.ContactEmail == "", "add their email to see what they sent to your forms")},
		{appInvoicing, func(ctx context.Context, sib *siblingConn) ([]elseItem, error) { return s.invoicesFor(ctx, sib, d) }, scopeInvoicesRead, whyNot(d.Company == "" && d.ContactName == "", "say who the deal is with to see their invoices")},
	}
	out := make([]elseSection, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		st := c.status(ctx, j.a)
		sec := elseSection{App: j.a.slug, Name: j.a.name, Items: []elseItem{}, Open: st.Open}
		if j.why != "" {
			sec.State, sec.Detail = "none", j.why
			out[i] = sec
			continue
		}
		if st.Via == "" || !st.Can(j.sc) {
			sec.State, sec.Detail, sec.Fix = "off", st.Detail, st.Fix
			out[i] = sec
			continue
		}
		sib, err := c.conn(ctx, j.a, j.sc)
		if err != nil {
			sec.State, sec.Detail = "off", err.Error()
			out[i] = sec
			continue
		}
		wg.Add(1)
		go func(i int, j job, sec elseSection, sib *siblingConn) {
			defer wg.Done()
			items, err := j.run(ctx, sib)
			if err != nil {
				sec.State, sec.Detail = "error", j.a.name+" did not answer: "+err.Error()
			} else {
				sec.State, sec.Items = "ok", items
			}
			out[i] = sec
		}(i, j, sec, sib)
	}
	wg.Wait()
	writeJSON(w, http.StatusOK, out)
}

func whyNot(cond bool, why string) string {
	if cond {
		return why
	}
	return ""
}

func linkTo(sib *siblingConn, path string) string {
	if sib == nil || sib.public == "" {
		return ""
	}
	return strings.TrimRight(sib.public, "/") + path
}

func (s *Server) mailWith(ctx context.Context, sib *siblingConn, d *store.Deal) ([]elseItem, error) {
	var out []elseItem
	seen := map[string]bool{}
	for _, q := range []string{"from:" + d.ContactEmail, "to:" + d.ContactEmail} {
		v, err := getJSONFrom(ctx, sib, "/api/v1/messages", url.Values{"q": {q}, "limit": {"15"}})
		if err != nil {
			return nil, err
		}
		for _, m := range rows(v, "messages") {
			id := idOf(m)
			if seen[id] {
				continue
			}
			seen[id] = true
			from := obj(m, "from")
			mine := !strings.EqualFold(str(from, "addr", "address", "email"), d.ContactEmail)
			out = append(out, elseItem{Kind: "email", Title: firstNonEmpty(str(m, "subject"), "(no subject)"), Body: clip(str(m, "snippet"), 240),
				At: str(m, "date"), URL: linkTo(sib, "/message/"+id), Out: mine, Ref: id})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	if len(out) > 20 {
		out = out[:20]
	}
	return out, nil
}

func (s *Server) meetingsWith(ctx context.Context, sib *siblingConn, d *store.Deal) ([]elseItem, error) {
	var out []elseItem
	seen := map[string]bool{}
	for _, q := range []string{d.ContactEmail, d.ContactName, d.Company, d.Title} {
		if strings.TrimSpace(q) == "" {
			continue
		}
		v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {q}})
		if err != nil {
			return nil, err
		}
		for _, e := range rows(v, "events", "items") {
			id := firstNonEmpty(str(e, "id"), idOf(e))
			if seen[id] {
				continue
			}
			seen[id] = true
			eid := firstNonEmpty(fmt.Sprint(int64(num(e, "event_id"))), id)
			out = append(out, elseItem{Kind: "meeting", Title: str(e, "title"), Body: clip(str(e, "location"), 120), At: str(e, "start"),
				URL: linkTo(sib, "/event/"+eid), Ref: id})
		}
		if len(out) >= 15 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].At > out[j].At })
	if len(out) > 15 {
		out = out[:15]
	}
	return out, nil
}

func (s *Server) answersFrom(ctx context.Context, sib *siblingConn, d *store.Deal) ([]elseItem, error) {
	v, err := getJSONFrom(ctx, sib, "/api/v1/responses", url.Values{"email": {d.ContactEmail}, "limit": {"10"}})
	if err != nil {
		return nil, err
	}
	var out []elseItem
	for _, r := range rows(v, "items", "responses") {
		out = append(out, elseItem{Kind: "answer", Title: firstNonEmpty(str(r, "form_title"), "A form"), Body: clip(str(r, "preview"), 240),
			At: str(r, "created_at"), URL: linkTo(sib, "/answers/"+idOf(r)), Ref: idOf(r)})
	}
	return out, nil
}

func (s *Server) invoicesFor(ctx context.Context, sib *siblingConn, d *store.Deal) ([]elseItem, error) {
	name := firstNonEmpty(d.Company, d.ContactName)
	v, err := getJSONFrom(ctx, sib, "/api/v1/invoices", url.Values{"q": {name}, "limit": {"15"}})
	if err != nil {
		return nil, err
	}
	var out []elseItem
	for _, inv := range rows(v, "items", "invoices") {
		if cn := str(inv, "client_name"); cn != "" && !strings.EqualFold(cn, name) && !strings.Contains(strings.ToLower(cn), strings.ToLower(name)) {
			continue
		}
		cents := int64(num(inv, "total")*100 + 0.5)
		out = append(out, elseItem{Kind: "invoice", Title: "Invoice " + str(inv, "number"), At: firstNonEmpty(str(inv, "issue_date"), str(inv, "created_at")),
			State: str(inv, "status"), Amount: fmtMoney(cents, firstNonEmpty(str(inv, "currency"), d.Currency)), URL: linkTo(sib, "/invoices/"+idOf(inv)), Ref: idOf(inv)})
	}
	return out, nil
}

// ---- leads from Pocket Forms ----

// handleLeads lists the answers to your forms that are not deals yet.
func (s *Server) handleLeads(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	c := s.clients(ctx, uid)
	st := c.status(ctx, appForms)
	if st.Via == "" || !st.Can(scopeFormsRead) {
		writeJSON(w, http.StatusOK, map[string]any{"state": st.State, "detail": st.Detail, "fix": st.Fix, "items": []any{}})
		return
	}
	sib, err := c.conn(ctx, appForms, scopeFormsRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "off", "detail": err.Error(), "items": []any{}})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/responses", url.Values{"status": {"open"}, "limit": {"60"}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"state": "error", "detail": "Pocket Forms did not answer: " + err.Error(), "items": []any{}})
		return
	}
	items := []map[string]any{}
	for _, x := range rows(v, "items", "responses") {
		id := idOf(x)
		item := map[string]any{"id": id, "form": str(x, "form_title"), "emoji": str(x, "form_emoji"), "name": str(x, "name"), "email": str(x, "email"),
			"preview": clip(str(x, "preview"), 300), "at": str(x, "created_at"), "status": str(x, "status"), "url": linkTo(sib, "/answers/"+id)}
		if d, err := s.store.DealBySource(ctx, uid, "forms:answer:"+id); err == nil {
			item["deal_id"], item["deal_title"] = d.ID, d.Title
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": "ok", "items": items, "open_url": st.Open})
}

// handleTakeLead turns one answer into a deal (once: again opens the same deal).
func (s *Server) handleTakeLead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	id := chiParam(r, "id")
	ref := "forms:answer:" + id
	if d, err := s.store.DealBySource(ctx, uid, ref); err == nil {
		writeJSON(w, http.StatusOK, d)
		return
	}
	var in struct {
		StageID int64 `json:"stage_id"`
	}
	_ = decode(r, &in)
	c := s.clients(ctx, uid)
	sib, err := c.conn(ctx, appForms, scopeFormsRead)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/responses/"+url.PathEscape(id), url.Values{"peek": {"1"}})
	if err != nil {
		writeErr(w, http.StatusBadGateway, "Pocket Forms: "+err.Error())
		return
	}
	view, _ := v.(map[string]any)
	resp, form := obj(view, "response"), obj(view, "form")
	role := map[string]string{}
	var lines []string
	for _, it := range rows(map[string]any{"x": view["items"]}, "x") {
		t := strings.TrimSpace(str(it, "text"))
		if t == "" || str(it, "type") == "section" {
			continue
		}
		if k := str(it, "role"); k != "" && role[k] == "" {
			role[k] = t
		}
		lines = append(lines, fmt.Sprintf("**%s:** %s", str(it, "label"), t))
	}
	name := firstNonEmpty(str(resp, "name"), role["name"])
	email := firstNonEmpty(str(resp, "email"), role["email"])
	company := role["company"]
	title := firstNonEmpty(role["subject"], str(form, "title"))
	if who := firstNonEmpty(company, name); who != "" {
		title = who + " — " + title
	}
	desc := strings.Join(lines, "\n\n")
	if u := linkTo(sib, "/answers/"+id); u != "" {
		desc += "\n\n— From Pocket Forms: " + u
	}
	src, t, phone := "form", clip(title, 200), role["phone"]
	in2 := dealIn{Title: &t, ContactName: &name, ContactEmail: &email, ContactPhone: &phone, Company: &company, Description: &desc, Source: &src,
		Note: "From the form “" + str(form, "title") + "”"}
	if amt := parseAmount(role["budget"]); amt > 0 {
		in2.Amount = &amt
	}
	if dl := role["deadline"]; validDate(dl) && dl != "" {
		in2.ExpectedClose = &dl
	}
	if in.StageID > 0 {
		in2.StageID = &in.StageID
	}
	step := "Get back to " + firstNonEmpty(name, company, "them")
	due := store.AddDays(today(), max(c.st.ActionDays, 0))
	in2.NextAction, in2.NextDue = &step, &due
	d, err := s.createDeal(ctx, uid, in2, "", ref)
	if err != nil {
		s.userFail(w, err, "lead")
		return
	}
	_ = s.store.AddMade(ctx, uid, &store.Made{DealID: d.ID, Target: "answer", App: "forms", Ref: id, Title: "Answer to “" + str(form, "title") + "”", URL: linkTo(sib, "/answers/"+id)})
	writeJSON(w, http.StatusCreated, d)
}

// parseAmount reads "5.000 €", "about 3000", "€2,500" as a number (0 = none).
func parseAmount(v string) float64 {
	var digits strings.Builder
	seenDigit := false
	for _, r := range v {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
			seenDigit = true
		case (r == '.' || r == ',') && seenDigit:
			digits.WriteRune(r)
		case seenDigit && r != ' ' && r != ' ':
			goto done
		}
	}
done:
	s := digits.String()
	if s == "" {
		return 0
	}
	// "5.000" and "2,500" are thousands; "12,50" / "12.50" are decimals
	last := strings.LastIndexAny(s, ".,")
	if last >= 0 && len(s)-last-1 == 3 {
		s = strings.NewReplacer(".", "", ",", "").Replace(s)
	} else if last >= 0 {
		s = strings.NewReplacer(".", "", ",", "").Replace(s[:last]) + "." + s[last+1:]
	}
	var f float64
	if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
		return 0
	}
	return f
}

// ---- keeping up with the other apps ----

// syncAll looks, for everyone, at what deals have out in the other apps.
func (s *Server) syncAll(ctx context.Context) {
	if !s.syncMu.TryLock() {
		return
	}
	defer s.syncMu.Unlock()
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	for _, uid := range ids {
		s.syncUser(ctx, uid)
	}
}

// syncUser checks up to 20 linked things: a document signed or declined,
// an invoice paid, a next-step task ticked off in Pocket Tasks.
func (s *Server) syncUser(ctx context.Context, uid int64) {
	list, err := s.store.MadeToCheck(ctx, uid, 20)
	if err != nil || len(list) == 0 {
		return
	}
	c := s.clients(ctx, uid)
	for _, m := range list {
		var (
			a     pocketApp
			scope string
			path  string
		)
		switch m.Target {
		case "signing":
			a, scope, path = appSign, scopeSigningRead, "/api/v1/documents/"+url.PathEscape(m.Ref)
		case "invoice":
			a, scope, path = appInvoicing, scopeInvoicesRead, "/api/v1/invoices/"+url.PathEscape(m.Ref)
		case "task":
			a, scope, path = appTasks, scopeTasksRead, "/api/v1/tasks/"+url.PathEscape(m.Ref)
		default:
			continue
		}
		sib, err := optional(c.conn(ctx, a, scope))
		if err != nil || sib == nil {
			_ = s.store.SetMadeState(ctx, uid, m.ID, m.State, "")
			continue
		}
		v, err := getJSONFrom(ctx, sib, path, nil)
		if err != nil {
			_ = s.store.SetMadeState(ctx, uid, m.ID, m.State, "")
			continue
		}
		obj0, _ := v.(map[string]any)
		if doc := obj(obj0, "document"); doc != nil {
			obj0 = doc
		}
		if t := obj(obj0, "task"); t != nil {
			obj0 = t
		}
		state := strings.ToLower(str(obj0, "status"))
		if state == "" || state == m.State {
			_ = s.store.SetMadeState(ctx, uid, m.ID, m.State, "")
			continue
		}
		_ = s.store.SetMadeState(ctx, uid, m.ID, state, "")
		s.madeChanged(ctx, uid, m, state)
	}
}

// madeChanged writes what changed elsewhere on the deal's timeline.
func (s *Server) madeChanged(ctx context.Context, uid int64, m *store.Made, state string) {
	line := ""
	switch m.Target {
	case "signing":
		switch state {
		case "completed":
			line = "Signed: " + m.Title
		case "declined":
			line = "Declined to sign: " + m.Title
		case "voided", "expired":
			line = "Signing " + state + ": " + m.Title
		}
	case "invoice":
		switch state {
		case "paid":
			line = "Invoice paid: " + m.Title
		case "sent":
			line = "Invoice sent: " + m.Title
		case "overdue":
			line = "Invoice overdue: " + m.Title
		}
	case "task":
		if state == "completed" || state == "done" || state == "cancelled" {
			d, err := s.store.GetDeal(ctx, uid, m.DealID)
			if err == nil && d.NextTaskRef == m.Ref {
				line = "Done (ticked off in Pocket Tasks): " + firstNonEmpty(d.NextAction, m.Title)
				_ = s.store.SetNextStep(ctx, uid, d.ID, "", "", "")
				_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, DealID: d.ID, Kind: "step", Title: line, App: "tasks", URL: m.URL})
				return
			}
		}
	}
	if line != "" {
		_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, DealID: m.DealID, Kind: "made", Title: line, App: m.App, URL: m.URL})
	}
}

// handleSyncNow looks at one deal's linked things now (the deal page asks
// when it opens, so a signature shows without waiting).
func (s *Server) handleSyncNow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.ownDeal(w, r)
	if !ok {
		return
	}
	s.syncUser(ctx, d.UserID)
	made, _ := s.store.ListMade(ctx, d.UserID, d.ID)
	writeJSON(w, http.StatusOK, made)
}
