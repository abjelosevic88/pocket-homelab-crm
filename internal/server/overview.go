package server

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

// ---- totals, today, people, reports, search, a day ----

// money is a sum per currency (cents), the person's own currency first.
type money map[string]int64

func (m money) add(cur string, v int64) {
	if v != 0 {
		m[cur] += v
	}
}

type summary struct {
	Currency    string `json:"currency"`
	Open        int    `json:"open"`
	OpenValue   money  `json:"open_value"`
	Weighted    money  `json:"weighted"`
	DueToday    int    `json:"due_today"`
	Overdue     int    `json:"overdue"`
	NoStep      int    `json:"no_step"`
	Stale       int    `json:"stale"`
	WonMonth    int    `json:"won_month"`
	WonMonthV   money  `json:"won_month_value"`
	WonYear     int    `json:"won_year"`
	WonYearV    money  `json:"won_year_value"`
	LostYear    int    `json:"lost_year"`
	WinRate     int    `json:"win_rate"` // % of the deals closed in the last 12 months
	ClosingSoon int    `json:"closing_soon"`
	URL         string `json:"url"`
}

func (s *Server) summarize(st store.Settings, all []*store.Deal) summary {
	now := time.Now()
	t := now.Format("2006-01-02")
	month := now.Format("2006-01")
	yearAgo := now.AddDate(-1, 0, 0).UTC().Format(time.RFC3339Nano)
	soon := now.AddDate(0, 0, 30).Format("2006-01-02")
	out := summary{Currency: st.Currency, OpenValue: money{}, Weighted: money{}, WonMonthV: money{}, WonYearV: money{}, URL: strings.TrimRight(s.cfg.BaseURL, "/") + "/"}
	won12, lost12 := 0, 0
	for _, d := range all {
		switch d.Status {
		case store.StatusOpen:
			out.Open++
			out.OpenValue.add(d.Currency, d.Value)
			out.Weighted.add(d.Currency, d.Weighted)
			switch {
			case d.NextDue != "" && d.NextDue < t:
				out.Overdue++
			case d.NextDue == t:
				out.DueToday++
			}
			if d.NextAction == "" {
				out.NoStep++
			}
			if d.Stale {
				out.Stale++
			}
			if d.ExpectedClose != "" && d.ExpectedClose <= soon {
				out.ClosingSoon++
			}
		case store.StatusWon:
			if d.ClosedAt >= yearAgo {
				won12++
				out.WonYear++
				out.WonYearV.add(d.Currency, d.Value)
			}
			if closedLocal(d.ClosedAt).Format("2006-01") == month {
				out.WonMonth++
				out.WonMonthV.add(d.Currency, d.Value)
			}
		case store.StatusLost:
			if d.ClosedAt >= yearAgo {
				lost12++
				out.LostYear++
			}
		}
	}
	if won12+lost12 > 0 {
		out.WinRate = won12 * 100 / (won12 + lost12)
	}
	return out
}

func closedLocal(v string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}
	}
	return t.Local()
}

func (s *Server) allDecorated(r *http.Request) (store.Settings, []*store.Deal, error) {
	ctx := r.Context()
	uid := actorID(ctx)
	st := s.settingsOf(ctx, uid)
	all, err := s.store.AllDeals(ctx, uid)
	if err != nil {
		return st, nil, err
	}
	decorate(st, all...)
	return st, all, nil
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	st, all, err := s.allDecorated(r)
	if err != nil {
		s.fail(w, err, "summary")
		return
	}
	writeJSON(w, http.StatusOK, s.summarize(st, all))
}

// handleToday is the Today page: what to do now and what needs a look.
func (s *Server) handleToday(w http.ResponseWriter, r *http.Request) {
	st, all, err := s.allDecorated(r)
	if err != nil {
		s.fail(w, err, "today")
		return
	}
	t := today()
	week := store.AddDays(t, 7)
	monthAgo := time.Now().AddDate(0, 0, -30).UTC().Format(time.RFC3339Nano)
	out := map[string][]*store.Deal{"overdue": {}, "today": {}, "week": {}, "no_step": {}, "stale": {}, "closing": {}, "won": {}}
	soon := store.AddDays(t, 14)
	for _, d := range all {
		switch d.Status {
		case store.StatusOpen:
			switch {
			case d.NextDue != "" && d.NextDue < t:
				out["overdue"] = append(out["overdue"], d)
			case d.NextDue == t || (d.NextAction != "" && d.NextDue == ""):
				out["today"] = append(out["today"], d)
			case d.NextDue != "" && d.NextDue <= week:
				out["week"] = append(out["week"], d)
			}
			if d.NextAction == "" {
				if d.Stale {
					out["stale"] = append(out["stale"], d)
				} else {
					out["no_step"] = append(out["no_step"], d)
				}
			}
			if d.ExpectedClose != "" && d.ExpectedClose <= soon {
				out["closing"] = append(out["closing"], d)
			}
		case store.StatusWon:
			if d.ClosedAt >= monthAgo {
				out["won"] = append(out["won"], d)
			}
		}
	}
	byDue := func(l []*store.Deal) {
		sort.SliceStable(l, func(i, j int) bool {
			if l[i].NextDue != l[j].NextDue {
				return l[i].NextDue < l[j].NextDue
			}
			return l[i].Value > l[j].Value
		})
	}
	byDue(out["overdue"])
	byDue(out["today"])
	byDue(out["week"])
	sort.SliceStable(out["stale"], func(i, j int) bool { return out["stale"][i].LastActivityAt < out["stale"][j].LastActivityAt })
	sort.SliceStable(out["closing"], func(i, j int) bool { return out["closing"][i].ExpectedClose < out["closing"][j].ExpectedClose })
	sort.SliceStable(out["won"], func(i, j int) bool { return out["won"][i].ClosedAt > out["won"][j].ClosedAt })
	writeJSON(w, http.StatusOK, map[string]any{"lists": out, "summary": s.summarize(st, all), "date": t})
}

// party is one person or company across their deals.
type party struct {
	Key        string   `json:"key"`
	Name       string   `json:"name"`
	Company    string   `json:"company"`
	Email      string   `json:"email"`
	Phone      string   `json:"phone"`
	ContactRef string   `json:"contact_ref"`
	CompanyRef string   `json:"company_ref"`
	Kind       string   `json:"kind"` // company | person
	Deals      int      `json:"deals"`
	Open       int      `json:"open"`
	OpenValue  money    `json:"open_value"`
	Won        int      `json:"won"`
	WonValue   money    `json:"won_value"`
	Lost       int      `json:"lost"`
	LastAt     string   `json:"last_at"`
	People     []string `json:"people"`
	DealIDs    []int64  `json:"deal_ids"`
}

// handlePeople groups deals by company (else by person): who you sell to.
func (s *Server) handlePeople(w http.ResponseWriter, r *http.Request) {
	_, all, err := s.allDecorated(r)
	if err != nil {
		s.fail(w, err, "people")
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	by := map[string]*party{}
	var order []string
	for _, d := range all {
		key, p := "", (*party)(nil)
		switch {
		case d.Company != "":
			key = "c:" + strings.ToLower(d.Company)
		case d.ContactEmail != "":
			key = "e:" + d.ContactEmail
		case d.ContactName != "":
			key = "n:" + strings.ToLower(d.ContactName)
		default:
			continue
		}
		if p = by[key]; p == nil {
			p = &party{Key: key, OpenValue: money{}, WonValue: money{}, People: []string{}, DealIDs: []int64{}}
			if d.Company != "" {
				p.Kind, p.Name, p.Company = "company", d.Company, d.Company
			} else {
				p.Kind, p.Name = "person", firstNonEmpty(d.ContactName, d.ContactEmail)
			}
			by[key] = p
			order = append(order, key)
		}
		p.Email, p.Phone = firstNonEmpty(p.Email, d.ContactEmail), firstNonEmpty(p.Phone, d.ContactPhone)
		p.ContactRef, p.CompanyRef = firstNonEmpty(p.ContactRef, d.ContactRef), firstNonEmpty(p.CompanyRef, d.CompanyRef)
		if d.ContactName != "" && !oneOf(d.ContactName, p.People) {
			p.People = append(p.People, d.ContactName)
		}
		p.Deals++
		p.DealIDs = append(p.DealIDs, d.ID)
		switch d.Status {
		case store.StatusOpen:
			p.Open++
			p.OpenValue.add(d.Currency, d.Value)
		case store.StatusWon:
			p.Won++
			p.WonValue.add(d.Currency, d.Value)
		case store.StatusLost:
			p.Lost++
		}
		if d.LastActivityAt > p.LastAt {
			p.LastAt = d.LastActivityAt
		}
	}
	out := []*party{}
	for _, k := range order {
		p := by[k]
		if q != "" && !strings.Contains(strings.ToLower(p.Name+" "+p.Email+" "+strings.Join(p.People, " ")), q) {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].LastAt > out[j].LastAt })
	writeJSON(w, http.StatusOK, out)
}

// handleLookup is a contact's page in another app: the deals with this
// email (or company).
func (s *Server) handleLookup(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	company := strings.TrimSpace(r.URL.Query().Get("company"))
	if email == "" && company == "" {
		writeErr(w, http.StatusBadRequest, "ask with ?email= or ?company=")
		return
	}
	st := s.settingsOf(ctx, uid)
	seen := map[int64]bool{}
	out := []*store.Deal{}
	for _, f := range []store.DealFilter{{Email: email}, {Company: company}} {
		if f.Email == "" && f.Company == "" {
			continue
		}
		f.Limit = 100
		list, _, err := s.store.ListDeals(ctx, uid, f)
		if err != nil {
			s.fail(w, err, "lookup")
			return
		}
		for _, d := range list {
			if !seen[d.ID] {
				seen[d.ID] = true
				out = append(out, d)
			}
		}
	}
	decorate(st, out...)
	for _, d := range out {
		d.Description = clip(d.Description, 300)
	}
	writeJSON(w, http.StatusOK, map[string]any{"deals": out, "url": strings.TrimRight(s.cfg.BaseURL, "/")})
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"deals": []any{}, "activities": []any{}})
		return
	}
	limit := queryInt(r, "limit")
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	list, total, err := s.store.ListDeals(ctx, uid, store.DealFilter{Search: q, Limit: limit})
	if err != nil {
		s.fail(w, err, "search")
		return
	}
	decorate(s.settingsOf(ctx, uid), list...)
	type hit struct {
		*store.Deal
		URL string `json:"url"`
	}
	out := []hit{}
	for _, d := range list {
		d.Description = clip(d.Description, 300)
		out = append(out, hit{d, s.dealLink(d.ID)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"deals": out, "total": total})
}

// handleDay is one day for a journal (Pocket Day): deals added, won and
// lost, steps done and talks had, and the steps due that day.
func (s *Server) handleDay(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	uid := actorID(ctx)
	date := chiParam(r, "date")
	day, err := time.ParseInLocation("2006-01-02", date, time.Local)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "day is YYYY-MM-DD")
		return
	}
	from := day.UTC().Format(time.RFC3339Nano)
	to := day.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano)
	acts, err := s.store.ActivitiesBetween(ctx, uid, from, to, 300)
	if err != nil {
		s.fail(w, err, "day")
		return
	}
	titles := map[int64]string{}
	type line struct {
		*store.Activity
		Deal    string `json:"deal"`
		DealURL string `json:"deal_url"`
	}
	lines := []line{}
	counts := map[string]int{}
	for _, a := range acts {
		if _, ok := titles[a.DealID]; !ok {
			if d, err := s.store.GetDeal(ctx, uid, a.DealID); err == nil {
				titles[a.DealID] = d.Title
			}
		}
		counts[a.Kind]++
		lines = append(lines, line{a, titles[a.DealID], s.dealLink(a.DealID)})
	}
	due, _, _ := s.store.ListDeals(ctx, uid, store.DealFilter{Status: store.StatusOpen, Due: "today", Today: date, Sort: "due", Limit: 50})
	dueOn := []*store.Deal{}
	for _, d := range due {
		if d.NextDue == date {
			dueOn = append(dueOn, d)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"date": date, "activities": lines, "counts": counts, "due": dueOn,
		"won": counts["won"], "lost": counts["lost"], "created": counts["created"], "url": strings.TrimRight(s.cfg.BaseURL, "/") + "/"})
}

// ---- reports ----

type monthRow struct {
	Month string `json:"month"` // YYYY-MM
	Won   int    `json:"won"`
	Lost  int    `json:"lost"`
	Added int    `json:"added"`
	Value int64  `json:"value"` // won, in the person's currency
}

type countRow struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
	Value int64  `json:"value"`
	Won   int    `json:"won"`
}

func (s *Server) handleReports(w http.ResponseWriter, r *http.Request) {
	st, all, err := s.allDecorated(r)
	if err != nil {
		s.fail(w, err, "reports")
		return
	}
	months := queryInt(r, "months")
	if months <= 0 || months > 36 {
		months = 12
	}
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local).AddDate(0, -(months - 1), 0)
	rowsBy := map[string]*monthRow{}
	var monthsOut []*monthRow
	for i := 0; i < months; i++ {
		m := start.AddDate(0, i, 0).Format("2006-01")
		row := &monthRow{Month: m}
		rowsBy[m] = row
		monthsOut = append(monthsOut, row)
	}
	stages, _ := s.store.ListStages(r.Context(), actorID(r.Context()))
	funnel := []countRow{}
	stageIdx := map[int64]int{}
	for _, sg := range stages {
		if sg.Kind == store.StatusOpen {
			stageIdx[sg.ID] = len(funnel)
			funnel = append(funnel, countRow{Name: sg.Name})
		}
	}
	lost := map[string]*countRow{}
	sources := map[string]*countRow{}
	var daysToWin []float64
	var wonValues []int64
	other := map[string]bool{}
	for _, d := range all {
		if row := rowsBy[closedLocal(d.CreatedAt).Format("2006-01")]; row != nil {
			row.Added++
		}
		if d.Currency != st.Currency {
			other[d.Currency] = true
		}
		src := firstNonEmpty(d.Source, "not said")
		if sources[src] == nil {
			sources[src] = &countRow{Name: src}
		}
		sources[src].Count++
		switch d.Status {
		case store.StatusOpen:
			if i, ok := stageIdx[d.StageID]; ok {
				funnel[i].Count++
				if d.Currency == st.Currency {
					funnel[i].Value += d.Value
				}
			}
		case store.StatusWon:
			sources[src].Won++
			if d.Currency == st.Currency {
				sources[src].Value += d.Value
				wonValues = append(wonValues, d.Value)
			}
			if row := rowsBy[closedLocal(d.ClosedAt).Format("2006-01")]; row != nil {
				row.Won++
				if d.Currency == st.Currency {
					row.Value += d.Value
				}
			}
			if a, b := closedLocal(d.CreatedAt), closedLocal(d.ClosedAt); !a.IsZero() && !b.IsZero() && b.After(a) {
				daysToWin = append(daysToWin, b.Sub(a).Hours()/24)
			}
		case store.StatusLost:
			if row := rowsBy[closedLocal(d.ClosedAt).Format("2006-01")]; row != nil {
				row.Lost++
			}
			why := firstNonEmpty(d.LostReason, "No reason given")
			if lost[why] == nil {
				lost[why] = &countRow{Name: why}
			}
			lost[why].Count++
			if d.Currency == st.Currency {
				lost[why].Value += d.Value
			}
		}
	}
	list := func(m map[string]*countRow) []*countRow {
		out := []*countRow{}
		for _, v := range m {
			out = append(out, v)
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Count != out[j].Count {
				return out[i].Count > out[j].Count
			}
			return out[i].Name < out[j].Name
		})
		return out
	}
	avg := func(v []float64) int {
		if len(v) == 0 {
			return 0
		}
		t := 0.0
		for _, x := range v {
			t += x
		}
		return int(t/float64(len(v)) + 0.5)
	}
	var avgWon int64
	if len(wonValues) > 0 {
		var t int64
		for _, v := range wonValues {
			t += v
		}
		avgWon = t / int64(len(wonValues))
	}
	others := []string{}
	for c := range other {
		others = append(others, c)
	}
	sort.Strings(others)
	writeJSON(w, http.StatusOK, map[string]any{"currency": st.Currency, "other_currencies": others, "months": monthsOut, "funnel": funnel,
		"lost_reasons": list(lost), "sources": list(sources), "days_to_win": avg(daysToWin), "average_won": avgWon,
		"summary": s.summarize(st, all)})
}
