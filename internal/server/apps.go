package server

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

// ---- a deal in the other Pocket apps ----
//
// From one deal, as its owner (their own Core link or direct token): the
// person and company into Pocket Contacts (or linked to the cards there), the
// next step as a task in Pocket Tasks, a meeting in Pocket Calendar, the deal
// as a note in Pocket Notes, an email from Pocket Mail, a proposal or
// contract from a Pocket Sign template, and — once won — the project in
// Pocket Projects and a draft invoice in Pocket Invoicing. Each thing made is
// remembered with a link to it; a failed try says why in plain words.
//
// Nothing here is needed for the deal itself: when an app is not on the box,
// switched off, or too old, the deal keeps the person, the step, the meeting
// and the email text on its own, and the page says what to switch on.

// origin is the line that says where a thing came from.
func (s *Server) origin(d *store.Deal) string {
	return fmt.Sprintf("From Pocket CRM — “%s”: %s", d.Title, s.dealLink(d.ID))
}

// dealMarkdown is a deal as Markdown (a note, a project's description).
func (s *Server) dealMarkdown(ctx context.Context, d *store.Deal, timeline bool) string {
	var b strings.Builder
	if who := d.Who(); who != "" {
		fmt.Fprintf(&b, "**With:** %s\n\n", who)
	}
	if d.ContactEmail != "" || d.ContactPhone != "" {
		fmt.Fprintf(&b, "**Reach them:** %s\n\n", joinNonEmpty(" · ", d.ContactEmail, d.ContactPhone))
	}
	if d.Value > 0 {
		fmt.Fprintf(&b, "**Worth:** %s\n\n", fmtMoney(d.Value, d.Currency))
	}
	if d.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", d.Description)
	}
	if d.OutcomeNote != "" {
		fmt.Fprintf(&b, "**Agreed:** %s\n\n", d.OutcomeNote)
	}
	if timeline {
		acts, _ := s.store.ListActivities(ctx, d.UserID, d.ID, 60)
		if len(acts) > 0 {
			b.WriteString("### Timeline\n\n")
			for _, a := range acts {
				line := a.Title
				if a.Body != "" {
					line = joinNonEmpty(": ", a.Title, strings.ReplaceAll(clip(a.Body, 400), "\n", " "))
				}
				fmt.Fprintf(&b, "- %s — %s\n", day10(a.At), line)
			}
			b.WriteString("\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// fmtMoney is 450000 cents EUR as "4,500 EUR" (cents only when there are some).
func fmtMoney(cents int64, cur string) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole, frac := cents/100, cents%100
	s := fmt.Sprintf("%d", whole)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	if frac != 0 {
		s += fmt.Sprintf(".%02d", frac)
	}
	if neg {
		s = "-" + s
	}
	return s + " " + cur
}

type makeIn struct {
	Target     string `json:"target"` // contact | task | event | note | email | signing | project | invoice
	Title      string `json:"title"`
	Notes      string `json:"notes"`
	Date       string `json:"date"`
	Time       string `json:"time"`
	Minutes    int    `json:"minutes"`
	Location   string `json:"location"`
	Due        string `json:"due"`
	Subject    string `json:"subject"`
	Body       string `json:"body"`
	To         string `json:"to"`
	TemplateID int64  `json:"template_id"`
	TeamID     int64  `json:"team_id"`
	Send       bool   `json:"send"`
	Timeline   bool   `json:"timeline"`
	ContactRef string `json:"contact_ref"` // contact: link this card instead of looking
}

var makeTargets = map[string]pocketApp{"contact": appContacts, "task": appTasks, "event": appCalendar, "note": appNotes,
	"email": appMail, "signing": appSign, "project": appProjects, "invoice": appInvoicing}

// make makes one thing for a deal in another app and remembers it.
func (s *Server) make(ctx context.Context, c *clients, d *store.Deal, in makeIn) (*store.Made, error) {
	a, ok := makeTargets[in.Target]
	if !ok {
		return nil, &userErr{"make a contact, task, meeting, note, email, document to sign, project or invoice"}
	}
	m := &store.Made{DealID: d.ID, Target: in.Target, App: a.slug}
	var (
		res  map[string]any
		path string
		err  error
	)
	switch in.Target {
	case "contact":
		return s.linkContact(ctx, c, d, in.ContactRef)
	case "task":
		res, path, err = s.makeTask(ctx, c, d, in)
	case "event":
		res, path, err = s.makeEvent(ctx, c, d, in)
	case "note":
		res, path, err = s.makeNote(ctx, c, d, in)
	case "email":
		err = s.sendEmail(ctx, c, d, in)
		m.Title = firstNonEmpty(in.Subject, "Email")
	case "signing":
		res, path, err = s.makeSigning(ctx, c, d, in)
	case "project":
		res, path, err = s.makeProject(ctx, c, d, in)
	case "invoice":
		res, path, err = s.makeInvoice(ctx, c, d, in)
	}
	if err != nil {
		return nil, err
	}
	if res != nil {
		m.Ref = idOf(res)
		m.Title = firstNonEmpty(str(res, "title", "name", "number", "fn"), in.Title, m.Title)
		m.State = str(res, "status")
		if in.Target == "invoice" && m.Title != "" && !strings.HasPrefix(m.Title, "Invoice") {
			m.Title = "Invoice " + m.Title
		}
		if sib, _ := c.conn(ctx, a, a.scopes[len(a.scopes)-1]); sib != nil && sib.public != "" && m.Ref != "" && path != "" {
			m.URL = strings.TrimRight(sib.public, "/") + path
		}
	}
	s.store.DropMadeErrors(ctx, d.UserID, d.ID, m.Target)
	if in.Target != "email" {
		if err := s.store.AddMade(ctx, d.UserID, m); err != nil {
			return nil, err
		}
	}
	s.madeLine(ctx, d, m, in)
	return m, nil
}

// madeLine writes what was made on the deal's timeline.
func (s *Server) madeLine(ctx context.Context, d *store.Deal, m *store.Made, in makeIn) {
	a := &store.Activity{UserID: d.UserID, DealID: d.ID, Kind: "made", App: m.App, URL: m.URL}
	switch m.Target {
	case "task":
		a.Title = "Task made: " + m.Title
	case "event":
		a.Kind, a.Title = "meeting", "Meeting planned: "+m.Title
		a.Body = joinNonEmpty(" ", in.Date, in.Time)
	case "note":
		a.Title = "Saved as a note"
	case "email":
		a.Kind, a.Title, a.Body = "email", "Email sent: "+firstNonEmpty(in.Subject, "(no subject)"), clip(in.Body, 4000)
	case "signing":
		a.Title = "Sent to sign: " + m.Title
		if !in.Send {
			a.Title = "Document made to sign: " + m.Title
		}
	case "project":
		a.Title = "Project started: " + m.Title
	case "invoice":
		a.Title = "Draft invoice made: " + m.Title
	default:
		return
	}
	_ = s.store.AddActivity(ctx, a)
}

// plainErr is a failed try in a sentence.
func plainErr(a pocketApp, err error) string {
	msg := err.Error()
	var ue *userErr
	if isNotConnected(err) || errors.As(err, &ue) {
		return msg
	}
	return a.name + ": " + msg
}

// ---- Pocket Contacts ----

// splitName turns "Ana Marić" into given and family names.
func splitName(n string) (given, family string) {
	parts := strings.Fields(n)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	}
	return strings.Join(parts[:len(parts)-1], " "), parts[len(parts)-1]
}

// findCompany is the company card with this name, if there is one.
func findCompany(ctx context.Context, sib *siblingConn, name string) map[string]any {
	if name == "" {
		return nil
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {name}, "limit": {"20"}})
	if err != nil {
		return nil
	}
	for _, c := range rows(v, "contacts") {
		if str(c, "kind") == "org" && strings.EqualFold(strings.TrimSpace(str(c, "fn", "org")), name) {
			return c
		}
	}
	return nil
}

// firstValue is the first email or phone of a card (strings or {value}).
func firstValue(v any) string {
	l, ok := v.([]any)
	if !ok || len(l) == 0 {
		return ""
	}
	switch x := l[0].(type) {
	case string:
		return x
	case map[string]any:
		return str(x, "value")
	}
	return ""
}

// linkContact links the deal to the person's card in Pocket Contacts: the
// one picked, the one with their email, or a new one (and the company's).
func (s *Server) linkContact(ctx context.Context, c *clients, d *store.Deal, pick string) (*store.Made, error) {
	if d.ContactName == "" && d.ContactEmail == "" && d.Company == "" && pick == "" {
		return nil, &userErr{"say who the deal is with first (a name, an email or a company)"}
	}
	rs, err := c.conn(ctx, appContacts, scopeContactsRead)
	if err != nil {
		return nil, err
	}
	m := &store.Made{DealID: d.ID, Target: "contact", App: appContacts.slug}
	cardURL := func(ref string) string {
		if rs.public == "" || ref == "" {
			return ""
		}
		return strings.TrimRight(rs.public, "/") + "/contacts/" + ref
	}
	link := func(card map[string]any) (*store.Made, error) {
		ref := idOf(card)
		contactRef, companyRef := ref, ""
		if str(card, "kind") == "org" {
			contactRef, companyRef = "", ref
		} else if org := findCompany(ctx, rs, d.Company); org != nil {
			companyRef = idOf(org)
		}
		_ = s.store.SetContactRef(ctx, d.UserID, d.ID, contactRef, companyRef)
		m.Ref, m.Title, m.URL = ref, firstNonEmpty(str(card, "fn", "name", "display_name"), d.ContactName, d.Company), cardURL(ref)
		return m, nil
	}
	if pick != "" {
		v, err := getJSONFrom(ctx, rs, "/api/v1/contacts/"+url.PathEscape(pick), nil)
		if err != nil {
			return nil, err
		}
		card, _ := v.(map[string]any)
		if card == nil {
			return nil, &userErr{"that contact is not there any more"}
		}
		if inner := obj(card, "card"); inner != nil {
			inner["id"] = card["id"]
			if k := str(card, "kind"); k != "" {
				inner["kind"] = k
			}
			card = inner
		}
		// the card's details fill what the deal does not know yet
		changed := false
		fill := func(dst *string, v string) {
			if *dst == "" && v != "" {
				*dst, changed = v, true
			}
		}
		if str(card, "kind") == "org" {
			fill(&d.Company, str(card, "fn", "org"))
		} else {
			fill(&d.ContactName, str(card, "fn"))
			fill(&d.Company, str(card, "org"))
			fill(&d.ContactEmail, strings.ToLower(firstValue(card["emails"])))
			fill(&d.ContactPhone, firstValue(card["phones"]))
		}
		if changed {
			_ = s.store.UpdateDeal(ctx, d)
		}
		return link(card)
	}
	if d.ContactEmail != "" {
		if v, err := getJSONFrom(ctx, rs, "/api/v1/lookup", url.Values{"email": {d.ContactEmail}}); err == nil {
			if found := rows(v, "contacts"); len(found) > 0 {
				return link(found[0])
			}
		}
	}
	if d.ContactName == "" && d.ContactEmail == "" {
		if org := findCompany(ctx, rs, d.Company); org != nil {
			return link(org)
		}
	}
	ws, err := c.conn(ctx, appContacts, scopeContactsWrite)
	if err != nil {
		return nil, err
	}
	companyRef := ""
	if d.Company != "" {
		if org := findCompany(ctx, rs, d.Company); org != nil {
			companyRef = idOf(org)
		} else if res, err := postJSONTo(ctx, ws, "/api/v1/contacts", map[string]any{"card": map[string]any{"kind": "org", "fn": d.Company, "org": d.Company, "note": s.origin(d)}}); err == nil {
			companyRef = idOf(res)
		}
	}
	if d.ContactName == "" && d.ContactEmail == "" {
		if companyRef == "" {
			return nil, &userErr{"Pocket Contacts did not take the company"}
		}
		_ = s.store.SetContactRef(ctx, d.UserID, d.ID, "", companyRef)
		m.Ref, m.Title, m.URL = companyRef, d.Company+" (added)", cardURL(companyRef)
		return m, nil
	}
	given, family := splitName(d.ContactName)
	card := map[string]any{"kind": "individual", "fn": firstNonEmpty(d.ContactName, d.ContactEmail), "given": given, "family": family, "org": d.Company, "note": s.origin(d)}
	if d.ContactEmail != "" {
		card["emails"] = []map[string]any{{"label": "Work", "value": d.ContactEmail, "pref": true}}
	}
	if d.ContactPhone != "" {
		card["phones"] = []map[string]any{{"label": "Work", "value": d.ContactPhone}}
	}
	res, err := postJSONTo(ctx, ws, "/api/v1/contacts", map[string]any{"card": card})
	if err != nil {
		return nil, err
	}
	ref := idOf(res)
	_ = s.store.SetContactRef(ctx, d.UserID, d.ID, ref, companyRef)
	m.Ref, m.Title, m.URL = ref, firstNonEmpty(str(res, "fn", "name", "display_name"), d.ContactName, d.ContactEmail)+" (added)", cardURL(ref)
	_ = s.store.AddActivity(ctx, &store.Activity{UserID: d.UserID, DealID: d.ID, Kind: "made", Title: "Added to Pocket Contacts: " + firstNonEmpty(d.ContactName, d.ContactEmail), App: "contacts", URL: m.URL})
	return m, nil
}

// handleContactSearch is the person picker on a deal: cards from Pocket Contacts.
func (s *Server) handleContactSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}})
		return
	}
	sib, err := c.conn(ctx, appContacts, scopeContactsRead)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}, "off": err.Error()})
		return
	}
	v, err := getJSONFrom(ctx, sib, "/api/v1/search", url.Values{"q": {q}, "limit": {"12"}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"contacts": []any{}, "off": "Pocket Contacts: " + err.Error()})
		return
	}
	out := []map[string]any{}
	for _, x := range rows(v, "contacts") {
		out = append(out, map[string]any{"id": idOf(x), "kind": str(x, "kind"), "name": str(x, "fn"), "company": str(x, "org"), "title": str(x, "title"),
			"email": firstValue(x["emails"]), "phone": firstValue(x["phones"]), "initials": str(x, "initials"), "color": str(x, "color")})
	}
	writeJSON(w, http.StatusOK, map[string]any{"contacts": out})
}

// ---- Pocket Tasks ----

func (s *Server) makeTask(ctx context.Context, c *clients, d *store.Deal, in makeIn) (map[string]any, string, error) {
	sib, err := c.conn(ctx, appTasks, scopeTasksWrite)
	if err != nil {
		return nil, "", err
	}
	title := firstNonEmpty(strings.TrimSpace(in.Title), d.NextAction, "Follow up with "+firstNonEmpty(d.Who(), d.Title))
	if !strings.Contains(title, d.Title) {
		title += " — " + d.Title
	}
	body := map[string]any{"title": clip(title, 200), "notes": clip(joinNonEmpty("\n\n", in.Notes, s.origin(d)), 20000), "url": s.dealLink(d.ID)}
	if due := firstNonEmpty(in.Due, in.Date); due != "" {
		if !validDate(due) {
			return nil, "", &userErr{"the due day is YYYY-MM-DD"}
		}
		body["due"] = due
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/tasks", body)
	if err != nil {
		return nil, "", err
	}
	return res, "/today?task=" + idOf(res), nil
}

// mirrorStep keeps the deal's next step as a task in Pocket Tasks (when
// the person wants that and Tasks is connected): made once, then renamed
// and moved with the step.
func (s *Server) mirrorStep(ctx context.Context, uid, dealID int64) {
	d, err := s.store.GetDeal(ctx, uid, dealID)
	if err != nil || d.Status != store.StatusOpen || d.NextAction == "" {
		return
	}
	c := s.clients(ctx, uid)
	if d.NextTaskRef != "" {
		sib, err := optional(c.conn(ctx, appTasks, scopeTasksWrite))
		if err != nil || sib == nil {
			return
		}
		body := map[string]any{"title": clip(d.NextAction+" — "+d.Title, 200)}
		if d.NextDue != "" {
			body["due"] = d.NextDue
		}
		var out map[string]any
		if err := sib.client.Do(ctx, http.MethodPut, "/api/v1/tasks/"+url.PathEscape(d.NextTaskRef), nil, body, &out); err != nil {
			s.log.Debug("tasks: update step", "deal", d.ID, "err", err)
		}
		return
	}
	if !c.st.TaskMirror {
		return
	}
	res, path, err := s.makeTask(ctx, c, d, makeIn{Title: d.NextAction, Due: d.NextDue})
	if err != nil {
		if !isNotConnected(err) {
			s.log.Debug("tasks: mirror step", "deal", d.ID, "err", err)
		}
		return
	}
	ref := idOf(res)
	_ = s.store.SetNextStep(ctx, uid, d.ID, d.NextAction, d.NextDue, ref)
	m := &store.Made{DealID: d.ID, Target: "task", App: "tasks", Ref: ref, Title: d.NextAction, State: "open"}
	if sib, _ := c.conn(ctx, appTasks, scopeTasksWrite); sib != nil && sib.public != "" {
		m.URL = strings.TrimRight(sib.public, "/") + path
	}
	_ = s.store.AddMade(ctx, uid, m)
}

// completeTask ticks a task off in Pocket Tasks (the step was done here).
func (s *Server) completeTask(ctx context.Context, uid int64, ref string) {
	c := s.clients(ctx, uid)
	sib, err := optional(c.conn(ctx, appTasks, scopeTasksWrite))
	if err != nil || sib == nil {
		return
	}
	var out map[string]any
	if err := sib.client.Do(ctx, http.MethodPost, "/api/v1/tasks/"+url.PathEscape(ref)+"/complete", nil, map[string]any{}, &out); err != nil {
		s.log.Debug("tasks: complete", "task", ref, "err", err)
	}
}

// ---- Pocket Calendar ----

// meetingTimes reads the day, time and length of a meeting.
func meetingTimes(in makeIn, defMinutes int) (start, end time.Time, allDay bool, err error) {
	if in.Date == "" || !validDate(in.Date) {
		return start, end, false, &userErr{"pick the day of the meeting"}
	}
	if in.Time == "" {
		start, _ = time.ParseInLocation("2006-01-02", in.Date, time.Local)
		return start, start.AddDate(0, 0, 1), true, nil
	}
	if !reTime.MatchString(in.Time) {
		return start, end, false, &userErr{"the time is HH:MM"}
	}
	start, err = time.ParseInLocation("2006-01-02 15:04", in.Date+" "+in.Time, time.Local)
	if err != nil {
		return start, end, false, &userErr{"the time is HH:MM"}
	}
	mins := in.Minutes
	if mins <= 0 {
		mins = max(defMinutes, 15)
	}
	return start, start.Add(time.Duration(mins) * time.Minute), false, nil
}

func meetingTitle(d *store.Deal, in makeIn) string {
	return clip(firstNonEmpty(strings.TrimSpace(in.Title), "Meeting with "+firstNonEmpty(d.Who(), d.Title)), 200)
}

func (s *Server) makeEvent(ctx context.Context, c *clients, d *store.Deal, in makeIn) (map[string]any, string, error) {
	start, end, allDay, err := meetingTimes(in, c.st.MeetingMinutes)
	if err != nil {
		return nil, "", err
	}
	sib, err := c.conn(ctx, appCalendar, scopeCalendarWrite)
	if err != nil {
		return nil, "", err
	}
	body := map[string]any{"title": meetingTitle(d, in), "description": joinNonEmpty("\n\n", in.Notes, d.Title, s.origin(d)), "tz": time.Local.String(),
		"location": clip(in.Location, 300), "url": s.dealLink(d.ID)}
	if allDay {
		body["start"], body["all_day"] = in.Date, true
	} else {
		body["start"], body["end"] = start.Format("2006-01-02T15:04"), end.Format("2006-01-02T15:04")
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/events", body)
	if err != nil {
		return nil, "", err
	}
	return res, "/event/" + idOf(res), nil
}

// handleMeetingICS is a meeting as a calendar file, for when Pocket
// Calendar is not connected: any calendar app opens it.
func (s *Server) handleMeetingICS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	d, err := s.store.GetDeal(ctx, uid, idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "deal")
		return
	}
	q := r.URL.Query()
	in := makeIn{Title: q.Get("title"), Date: q.Get("date"), Time: q.Get("time"), Minutes: queryInt(r, "minutes"), Location: q.Get("location"), Notes: q.Get("notes")}
	start, end, allDay, err := meetingTimes(in, s.settingsOf(ctx, uid).MeetingMinutes)
	if err != nil {
		s.userFail(w, err, "meeting")
		return
	}
	esc := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`).Replace
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Pocket CRM//EN\r\nBEGIN:VEVENT\r\n")
	fmt.Fprintf(&b, "UID:crm-%d-%d@pocket-crm\r\nDTSTAMP:%s\r\n", d.ID, start.Unix(), time.Now().UTC().Format("20060102T150405Z"))
	if allDay {
		fmt.Fprintf(&b, "DTSTART;VALUE=DATE:%s\r\nDTEND;VALUE=DATE:%s\r\n", start.Format("20060102"), end.Format("20060102"))
	} else {
		fmt.Fprintf(&b, "DTSTART:%s\r\nDTEND:%s\r\n", start.UTC().Format("20060102T150405Z"), end.UTC().Format("20060102T150405Z"))
	}
	fmt.Fprintf(&b, "SUMMARY:%s\r\nDESCRIPTION:%s\r\n", esc(meetingTitle(d, in)), esc(joinNonEmpty("\n\n", in.Notes, s.origin(d))))
	if in.Location != "" {
		fmt.Fprintf(&b, "LOCATION:%s\r\n", esc(in.Location))
	}
	if d.ContactEmail != "" {
		fmt.Fprintf(&b, "ATTENDEE;CN=%s:mailto:%s\r\n", esc(firstNonEmpty(d.ContactName, d.ContactEmail)), d.ContactEmail)
	}
	b.WriteString("END:VEVENT\r\nEND:VCALENDAR\r\n")
	if queryBool(r, "log") {
		_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, DealID: d.ID, Kind: "meeting", Title: "Meeting planned: " + meetingTitle(d, in), Body: joinNonEmpty(" ", in.Date, in.Time)})
	}
	w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="meeting-%d.ics"`, d.ID))
	_, _ = w.Write([]byte(b.String()))
}

// ---- Pocket Notes ----

func (s *Server) makeNote(ctx context.Context, c *clients, d *store.Deal, in makeIn) (map[string]any, string, error) {
	sib, err := c.conn(ctx, appNotes, scopeNotesWrite)
	if err != nil {
		return nil, "", err
	}
	title := firstNonEmpty(strings.TrimSpace(in.Title), d.Title)
	content := joinNonEmpty("\n\n", in.Notes, s.dealMarkdown(ctx, d, true), "— "+s.origin(d))
	res, err := postJSONTo(ctx, sib, "/api/v1/notes", map[string]any{"title": clip(title, 200), "content": content})
	if err != nil {
		return nil, "", err
	}
	return res, "/note/" + idOf(res), nil
}

// ---- Pocket Mail ----

func mailHTML(body string) string {
	var h strings.Builder
	h.WriteString(`<!doctype html><html><head><meta charset="utf-8"></head><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2433;line-height:1.55">`)
	for _, p := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n\n") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		h.WriteString(`<p style="margin:0 0 12px">` + strings.ReplaceAll(html.EscapeString(strings.TrimSpace(p)), "\n", "<br>") + `</p>`)
	}
	h.WriteString(`</body></html>`)
	return h.String()
}

func (s *Server) sendEmail(ctx context.Context, c *clients, d *store.Deal, in makeIn) error {
	to := strings.ToLower(firstNonEmpty(strings.TrimSpace(in.To), d.ContactEmail))
	if to == "" || !reEmail.MatchString(to) {
		return &userErr{"this deal has no email address to write to"}
	}
	body := strings.TrimSpace(in.Body)
	if body == "" {
		return &userErr{"write the email first"}
	}
	subject := clip(firstNonEmpty(strings.TrimSpace(in.Subject), d.Title), 200)
	if s.cfg.DemoData {
		s.log.Debug("demo: email not sent", "to", to, "subject", subject)
		return nil
	}
	sib, err := c.conn(ctx, appMail, scopeMailSend)
	if err != nil {
		return err
	}
	msg := map[string]any{"from": c.st.MailFrom, "to": []map[string]string{{"name": d.ContactName, "addr": to}}, "subject": subject,
		"body_text": body, "body_html": mailHTML(body)}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var out map[string]any
	return sib.client.Do(ctx, http.MethodPost, "/api/v1/send", nil, msg, &out)
}

// ---- Pocket Sign ----

func (s *Server) makeSigning(ctx context.Context, c *clients, d *store.Deal, in makeIn) (map[string]any, string, error) {
	if d.ContactEmail == "" {
		return nil, "", &userErr{"this deal has no email address to send the document to"}
	}
	if in.TemplateID <= 0 {
		return nil, "", &userErr{"pick a template from Pocket Sign"}
	}
	rs, err := c.conn(ctx, appSign, scopeSigningRead)
	if err != nil {
		return nil, "", err
	}
	v, err := getJSONFrom(ctx, rs, "/api/v1/templates", nil)
	if err != nil {
		return nil, "", err
	}
	var roles []map[string]any
	for _, t := range rows(v, "templates") {
		if int64(num(t, "id")) == in.TemplateID {
			roles = rows(map[string]any{"items": t["roles"]})
		}
	}
	if len(roles) == 0 {
		return nil, "", &userErr{"that Pocket Sign template has nobody to sign it"}
	}
	pick := roles[0]
	for _, rl := range roles {
		if str(rl, "email") == "" {
			pick = rl
			break
		}
	}
	people := []map[string]any{{"recipient_id": int64(num(pick, "id")), "name": firstNonEmpty(d.ContactName, d.ContactEmail), "email": d.ContactEmail}}
	ws, err := c.conn(ctx, appSign, scopeSigningWrite)
	if err != nil {
		return nil, "", err
	}
	res, err := postJSONTo(ctx, ws, fmt.Sprintf("/api/v1/templates/%d/use", in.TemplateID),
		map[string]any{"title": clip(strings.TrimSpace(in.Title), 200), "message": strings.TrimSpace(in.Notes), "people": people, "send": in.Send})
	if err != nil {
		return nil, "", err
	}
	if doc, ok := res["document"].(map[string]any); ok {
		res = doc
	}
	return res, "/documents/" + idOf(res), nil
}

// ---- Pocket Projects ----

func (s *Server) makeProject(ctx context.Context, c *clients, d *store.Deal, in makeIn) (map[string]any, string, error) {
	sib, err := c.conn(ctx, appProjects, scopeProjectsWrite)
	if err != nil {
		return nil, "", err
	}
	name := strings.TrimSpace(in.Title)
	if name == "" {
		name = d.Title
		if d.Company != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(d.Company)) {
			name = d.Company + " — " + name
		}
	}
	content := joinNonEmpty("\n\n", strings.TrimSpace(in.Notes), s.dealMarkdown(ctx, d, false), "---\n"+s.origin(d))
	body := map[string]any{"name": clip(name, 200), "summary": clip(firstNonEmpty(d.OutcomeNote, strings.Join(strings.Fields(d.Description), " ")), 250),
		"content": content, "status": "planned"}
	team := in.TeamID
	if team == 0 {
		team = c.st.ProjectTeam
	}
	if team > 0 {
		body["team_ids"] = []int64{team}
	}
	res, err := postJSONTo(ctx, sib, "/api/v1/projects", body)
	if err != nil {
		if strings.Contains(err.Error(), "team") {
			return nil, "", &userErr{"Pocket Projects needs to know the team: pick one (Settings → Deals → Projects team)"}
		}
		return nil, "", err
	}
	return res, "/project/" + idOf(res), nil
}

// ---- Pocket Invoicing ----

func (s *Server) makeInvoice(ctx context.Context, c *clients, d *store.Deal, in makeIn) (map[string]any, string, error) {
	if d.Value <= 0 {
		return nil, "", &userErr{"the deal has no value to invoice yet"}
	}
	if st := c.status(ctx, appInvoicing); st.Via != "" && !st.Can(scopeInvoicesWrite) {
		return nil, "", &notConnected{"Pocket Invoicing does not make invoices for CRM yet: " + firstNonEmpty(st.Fix, st.Detail)}
	}
	sib, err := c.conn(ctx, appInvoicing, scopeInvoicesWrite)
	if err != nil {
		return nil, "", err
	}
	name := firstNonEmpty(d.Company, d.ContactName, d.ContactEmail)
	if name == "" {
		return nil, "", &userErr{"say who the deal is with first: the invoice needs a client"}
	}
	clientID := int64(0)
	if v, err := getJSONFrom(ctx, sib, "/api/v1/clients", url.Values{"q": {name}}); err == nil {
		for _, cl := range rows(v, "items", "clients") {
			if strings.EqualFold(strings.TrimSpace(str(cl, "name")), name) || (d.ContactEmail != "" && strings.EqualFold(str(cl, "email"), d.ContactEmail)) {
				clientID = int64(num(cl, "id"))
				break
			}
		}
	}
	if clientID == 0 {
		cl, err := postJSONTo(ctx, sib, "/api/v1/clients", map[string]any{"name": name, "contact_name": d.ContactName, "email": d.ContactEmail, "phone": d.ContactPhone})
		if err != nil {
			return nil, "", err
		}
		clientID = int64(num(cl, "id"))
	}
	desc := firstNonEmpty(strings.TrimSpace(in.Title), d.Title)
	body := map[string]any{"client_id": clientID, "currency": d.Currency, "status": "draft",
		"notes": clip(joinNonEmpty("\n", strings.TrimSpace(in.Notes), s.origin(d)), 4000),
		"items": []map[string]any{{"description": clip(desc, 300), "quantity": 1, "unit_price": float64(d.Value) / 100}}}
	res, err := postJSONTo(ctx, sib, "/api/v1/invoices", body)
	if err != nil {
		return nil, "", err
	}
	return res, "/invoices/" + idOf(res), nil
}

// ---- the handlers ----

func (s *Server) ownDeal(w http.ResponseWriter, r *http.Request) (*store.Deal, bool) {
	ctx := r.Context()
	d, err := s.store.GetDeal(ctx, actorID(ctx), idParam(r, "id"))
	if err != nil {
		s.fail(w, err, "deal")
		return nil, false
	}
	return d, true
}

func (s *Server) makeFail(w http.ResponseWriter, a pocketApp, err error) {
	var ue *userErr
	switch {
	case errors.As(err, &ue), isNotConnected(err):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
	}
}

func (s *Server) handleMake(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.ownDeal(w, r)
	if !ok {
		return
	}
	var in makeIn
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	c := s.clients(ctx, d.UserID)
	m, err := s.make(ctx, c, d, in)
	if err != nil {
		s.makeFail(w, makeTargets[in.Target], err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// handleWon wins a deal and takes it on: the project, a draft invoice and
// a first task — each step on its own, so one app being off does not stop
// the others (or the win).
func (s *Server) handleWon(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.ownDeal(w, r)
	if !ok {
		return
	}
	var in struct {
		StageID int64  `json:"stage_id"`
		Note    string `json:"note"`
		Project bool   `json:"project"`
		Invoice bool   `json:"invoice"`
		Task    bool   `json:"task"`
		Contact bool   `json:"contact"`
		TeamID  int64  `json:"team_id"`
		Name    string `json:"name"`
		Due     string `json:"due"`
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	uid := d.UserID
	if in.StageID == 0 {
		st, err := s.store.FirstStage(ctx, uid, store.StatusWon)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "there is no Won stage: add one in Settings → Pipeline")
			return
		}
		in.StageID = st.ID
	}
	if sg, err := s.store.GetStage(ctx, uid, in.StageID); err != nil || sg.Kind != store.StatusWon {
		writeErr(w, http.StatusBadRequest, "that is not a Won stage")
		return
	}
	taskRef := d.NextTaskRef
	d, err := s.moveDeal(ctx, uid, d.ID, in.StageID, nil, nil, "", strings.TrimSpace(in.Note))
	if err != nil {
		s.userFail(w, err, "win deal")
		return
	}
	if taskRef != "" {
		go s.completeTask(context.WithoutCancel(ctx), uid, taskRef)
	}
	_ = s.store.SetNextStep(ctx, uid, d.ID, "", "", "")
	c := s.clients(ctx, uid)
	type step struct {
		Target string      `json:"target"`
		Made   *store.Made `json:"made,omitempty"`
		Error  string      `json:"error,omitempty"`
	}
	steps := []step{}
	run := func(mi makeIn) {
		m, err := s.make(ctx, c, d, mi)
		st := step{Target: mi.Target, Made: m}
		if err != nil {
			st.Error = plainErr(makeTargets[mi.Target], err)
		}
		steps = append(steps, st)
	}
	if in.Contact && d.ContactRef == "" && d.CompanyRef == "" {
		run(makeIn{Target: "contact"})
	}
	projectURL := ""
	if in.Project {
		run(makeIn{Target: "project", Title: in.Name, TeamID: in.TeamID})
		if last := steps[len(steps)-1]; last.Made != nil {
			projectURL = last.Made.URL
		}
	}
	if in.Invoice {
		run(makeIn{Target: "invoice"})
	}
	if in.Task {
		due := in.Due
		if due == "" {
			due = store.AddDays(today(), max(c.st.ActionDays, 1))
		}
		run(makeIn{Target: "task", Title: "Kick off " + d.Title, Notes: projectURL, Due: due})
	}
	d, _ = s.store.GetDeal(ctx, uid, d.ID)
	decorate(c.st, d)
	writeJSON(w, http.StatusOK, map[string]any{"steps": steps, "deal": d})
}

// handleApps says per app whether deals can reach it, and if not why and
// what to do — the pages grey out what does not work.
func (s *Server) handleApps(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	out := []appState{}
	for _, a := range pocketApps {
		out = append(out, c.status(ctx, a))
	}
	writeJSON(w, http.StatusOK, map[string]any{"apps": out, "core": s.core.Configured()})
}

// handleAppOptions lists what to pick from in an app: Projects' teams, Sign's templates.
func (s *Server) handleAppOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	c := s.clients(ctx, actorID(ctx))
	var (
		a     pocketApp
		scope string
		path  string
		key   string
	)
	switch chiParam(r, "what") {
	case "teams":
		a, scope, path, key = appProjects, scopeProjectsRead, "/api/v1/teams", "teams"
	case "templates":
		a, scope, path, key = appSign, scopeSigningRead, "/api/v1/templates", "templates"
	default:
		writeErr(w, http.StatusNotFound, "unknown list")
		return
	}
	sib, err := c.conn(ctx, a, scope)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v, err := getJSONFrom(ctx, sib, path, nil)
	if err != nil {
		writeErr(w, http.StatusBadGateway, a.name+": "+err.Error())
		return
	}
	out := []map[string]any{}
	for _, x := range rows(v, key) {
		o := map[string]any{"id": num(x, "id"), "name": str(x, "name", "title"), "key": str(x, "key")}
		if key == "templates" {
			o["roles"] = x["roles"]
		}
		out = append(out, o)
	}
	writeJSON(w, http.StatusOK, out)
}
