package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

func TestDealsArePerPerson(t *testing.T) {
	a := newApp(t, nil)
	d := a.ok(a.alice, "POST", "/api/v1/deals", map[string]any{"title": "Shop for the bakery", "amount": 4200, "contact_name": "Ana Marić", "contact_email": "Ana@Pekara.ba", "company": "Pekara Marić"})
	if d["contact_email"] != "ana@pekara.ba" || num(d, "value") != 420000 || d["status"] != "open" {
		t.Fatalf("deal as stored: %v", d)
	}
	path := "/api/v1/deals/" + id(d)
	// Bob sees none of it, and cannot touch it
	a.expect(404, a.bob, "GET", path, nil)
	a.expect(404, a.bob, "PUT", path, map[string]any{"title": "mine"})
	a.expect(404, a.bob, "DELETE", path, nil)
	a.expect(404, a.bob, "POST", path+"/activities", map[string]any{"title": "hello"})
	a.expect(404, a.bob, "POST", path+"/move", map[string]any{"stage_id": 1})
	a.expect(404, a.bob, "POST", path+"/make", map[string]any{"target": "note"})
	a.expect(404, a.bob, "GET", path+"/meeting.ics?date=2026-12-01", nil)
	if l := a.ok(a.bob, "GET", "/api/v1/deals", nil); num(l, "total") != 0 {
		t.Fatalf("Bob sees Alice's deals: %v", l)
	}
	if l := a.ok(a.bob, "GET", "/api/v1/lookup?email=ana@pekara.ba", nil); len(l["deals"].([]any)) != 0 {
		t.Fatalf("Bob's lookup finds Alice's deal")
	}
	if l := a.list(a.bob, "/api/v1/people"); len(l) != 0 {
		t.Fatalf("Bob sees Alice's people: %v", l)
	}
	// Bob's stages are his own; Alice's stage ids do not work for him
	aliceStages := a.list(a.alice, "/api/v1/stages")
	first := aliceStages[0].(map[string]any)
	a.expect(404, a.bob, "PUT", "/api/v1/stages/"+id(first), map[string]any{"name": "x"})
	a.expect(400, a.bob, "POST", "/api/v1/deals", map[string]any{"title": "x", "stage_id": num(first, "id")})
	// an activity id of Alice's is not Bob's
	act := a.ok(a.alice, "POST", path+"/activities", map[string]any{"kind": "call", "title": "Called Ana", "body": "Wants it by spring"})
	a.expect(404, a.bob, "PUT", "/api/v1/activities/"+id(act), map[string]any{"title": "x"})
	a.expect(404, a.bob, "DELETE", "/api/v1/activities/"+id(act), nil)
	// Alice sees it all
	got := a.ok(a.alice, "GET", path, nil)
	if len(got["activities"].([]any)) != 2 {
		t.Fatalf("timeline: %v", got["activities"])
	}
}

func TestPipelineMoveWinLose(t *testing.T) {
	a := newApp(t, nil)
	stages := a.list(a.alice, "/api/v1/stages")
	if len(stages) != len(store.DefaultStages) {
		t.Fatalf("default stages: %d", len(stages))
	}
	var won, lost, second map[string]any
	for i, s := range stages {
		st := s.(map[string]any)
		switch st["kind"] {
		case "won":
			won = st
		case "lost":
			lost = st
		}
		if i == 1 {
			second = st
		}
	}
	d := a.ok(a.alice, "POST", "/api/v1/deals", map[string]any{"title": "Booking site", "amount": 6400, "next_action": "Send the proposal", "next_due": "2026-01-01"})
	if d["overdue"] != true || num(d, "chance") != 10 {
		t.Fatalf("new deal: overdue %v chance %v", d["overdue"], d["chance"])
	}
	path := "/api/v1/deals/" + id(d)
	m := a.ok(a.alice, "POST", path+"/move", map[string]any{"stage_id": num(second, "id")})
	if num(m, "chance") != 25 || m["stage_name"] != second["name"] {
		t.Fatalf("moved: %v", m)
	}
	// the step done, the next one set
	m = a.ok(a.alice, "POST", path+"/step/done", map[string]any{"note": "Sent with two options", "next_action": "Follow up", "next_due": "2099-01-01"})
	if m["next_action"] != "Follow up" || m["overdue"] == true {
		t.Fatalf("after step: %v", m)
	}
	// lost with a reason, then opened again, then won through the Won flow
	m = a.ok(a.alice, "POST", path+"/move", map[string]any{"stage_id": num(lost, "id"), "lost_reason": "No budget"})
	if m["status"] != "lost" || m["lost_reason"] != "No budget" || m["closed_at"] == "" {
		t.Fatalf("lost: %v", m)
	}
	m = a.ok(a.alice, "POST", path+"/move", map[string]any{"stage_id": num(second, "id")})
	if m["status"] != "open" || m["closed_at"] != "" || m["lost_reason"] != "" {
		t.Fatalf("opened again: %v", m)
	}
	// the other apps are not there: winning still works, each step says why not
	res := a.ok(a.alice, "POST", path+"/won", map[string]any{"project": true, "invoice": true, "task": true, "note": "Signed for spring"})
	deal := res["deal"].(map[string]any)
	if deal["status"] != "won" || deal["next_action"] != "" || deal["outcome_note"] != "Signed for spring" || num(deal, "stage_id") != num(won, "id") {
		t.Fatalf("won: %v", deal)
	}
	for _, s := range res["steps"].([]any) {
		st := s.(map[string]any)
		if st["error"] == "" || st["error"] == nil {
			t.Fatalf("step %v worked without the app", st)
		}
	}
	full := a.ok(a.alice, "GET", path, nil)
	var kinds []string
	for _, x := range full["activities"].([]any) {
		kinds = append(kinds, x.(map[string]any)["kind"].(string))
	}
	if j := strings.Join(kinds, ","); !strings.Contains(j, "won") || !strings.Contains(j, "lost") || !strings.Contains(j, "step") {
		t.Fatalf("timeline kinds: %s", j)
	}
	sum := a.ok(a.alice, "GET", "/api/v1/summary", nil)
	if num(sum, "won_year") != 1 || num(sum, "open") != 0 {
		t.Fatalf("summary: %v", sum)
	}
	// a stage with deals in it is not deleted without saying where they go
	a.expect(409, a.alice, "DELETE", "/api/v1/stages/"+id(won), nil)
}

func TestAppsWithoutCoreAreExplained(t *testing.T) {
	a := newApp(t, nil)
	res := a.ok(a.alice, "GET", "/api/v1/apps", nil)
	apps := res["apps"].([]any)
	if len(apps) != len(pocketApps) {
		t.Fatalf("apps: %d", len(apps))
	}
	for _, x := range apps {
		st := x.(map[string]any)
		if st["via"] != "" || st["state"] == "on" || st["fix"] == "" {
			t.Fatalf("an app without Core or a token should say what to do: %v", st)
		}
	}
	d := a.ok(a.alice, "POST", "/api/v1/deals", map[string]any{"title": "Audit", "contact_email": "x@y.ba", "company": "Y"})
	path := "/api/v1/deals/" + id(d)
	for _, target := range []string{"contact", "task", "event", "note", "signing", "project", "invoice"} {
		body := map[string]any{"target": target, "date": "2099-01-02", "template_id": 1}
		m := a.expect(400, a.alice, "POST", path+"/make", body)
		if !strings.Contains(m["error"].(string), "Pocket") && !strings.Contains(m["error"].(string), "value") {
			t.Fatalf("%s: %v", target, m)
		}
	}
	// the timeline from the other apps greys out each part on its own
	_, _, l := a.call(a.alice, "GET", path+"/elsewhere", nil)
	if len(l) != 4 {
		t.Fatalf("elsewhere: %v", l)
	}
	for _, x := range l {
		if st := x.(map[string]any)["state"]; st == "ok" {
			t.Fatalf("an app answered without being connected: %v", x)
		}
	}
	// the fallback for a meeting: a calendar file that logs the meeting
	req, _ := http.NewRequest("GET", a.url+path+"/meeting.ics?date=2099-01-02&time=10:30&minutes=45&log=1", nil)
	req.Header.Set("Authorization", "Bearer "+a.alice)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	ics, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(ics, []byte("BEGIN:VEVENT")) || !bytes.Contains(ics, []byte("ATTENDEE;CN=x@y.ba:mailto:x@y.ba")) {
		t.Fatalf("ics: %s", ics)
	}
	leads := a.ok(a.alice, "GET", "/api/v1/leads", nil)
	if leads["state"] == "ok" || leads["fix"] == "" {
		t.Fatalf("leads without Forms: %v", leads)
	}
}

func TestExportImportIsIdempotent(t *testing.T) {
	a := newApp(t, nil)
	d := a.ok(a.alice, "POST", "/api/v1/deals", map[string]any{"title": "Wine club", "amount": 8900, "company": "Vinarija", "tags": []string{"shop", "#shop", "wine"}})
	if len(d["tags"].([]any)) != 2 {
		t.Fatalf("tags: %v", d["tags"])
	}
	a.ok(a.alice, "POST", "/api/v1/deals/"+id(d)+"/activities", map[string]any{"title": "Met them"})
	doc, _ := json.Marshal(a.s.export(context.Background(), a.ids["Alice"]))
	post := func(tok string, body []byte) map[string]any {
		req, _ := http.NewRequest("POST", a.url+"/api/v1/import", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var m map[string]any
		_ = json.NewDecoder(res.Body).Decode(&m)
		if res.StatusCode != 200 {
			t.Fatalf("import → %d %v", res.StatusCode, m)
		}
		return m
	}
	// into Bob's account: once added, twice nothing new
	if m := post(a.bob, doc); num(m, "deals") != 1 {
		t.Fatalf("first import: %v", m)
	}
	if m := post(a.bob, doc); num(m, "deals") != 0 || num(m, "skipped") != 1 {
		t.Fatalf("second import: %v", m)
	}
	bobs := a.ok(a.bob, "GET", "/api/v1/deals", nil)["items"].([]any)
	if len(bobs) != 1 {
		t.Fatalf("Bob's deals: %v", bobs)
	}
	full := a.ok(a.bob, "GET", "/api/v1/deals/"+id(bobs[0].(map[string]any)), nil)
	if len(full["activities"].([]any)) != 2 {
		t.Fatalf("imported timeline: %v", full["activities"])
	}
	// a Pipedrive-like CSV, twice
	csv := []byte("Deal Title;Value;Currency;Organization;Person;Email;Stage;Status;Lost reason\nNew shop;\"5.000\";EUR;Pekara;Ana;ana@p.ba;Proposal sent;open;\nOld site;2,500;EUR;Atlas;Lejla;;Negotiating;lost;No budget\n")
	if m := post(a.alice, csv); num(m, "deals") != 2 {
		t.Fatalf("csv: %v", m)
	}
	if m := post(a.alice, csv); num(m, "deals") != 0 {
		t.Fatalf("csv again: %v", m)
	}
	l := a.ok(a.alice, "GET", "/api/v1/deals?q=New+shop", nil)["items"].([]any)
	if len(l) != 1 || num(l[0].(map[string]any), "value") != 500000 || l[0].(map[string]any)["stage_name"] != "Proposal sent" {
		t.Fatalf("csv deal: %v", l)
	}
	lostOne := a.ok(a.alice, "GET", "/api/v1/deals?status=lost", nil)["items"].([]any)
	if len(lostOne) != 1 || lostOne[0].(map[string]any)["lost_reason"] != "No budget" {
		t.Fatalf("csv lost: %v", lostOne)
	}
}

func TestSettingsArePerPerson(t *testing.T) {
	a := newApp(t, nil)
	a.ok(a.alice, "PUT", "/api/v1/settings", map[string]any{"currency": "km", "stale_days": 21, "lost_reasons": []string{"Too far", "Too far", " "}})
	al := a.ok(a.alice, "GET", "/api/v1/settings", nil)
	if al["currency"] != "BAM" || num(al, "stale_days") != 21 || len(al["lost_reasons"].([]any)) != 1 {
		t.Fatalf("alice: %v %v %v", al["currency"], al["stale_days"], al["lost_reasons"])
	}
	bo := a.ok(a.bob, "GET", "/api/v1/settings", nil)
	if bo["currency"] != "EUR" || num(bo, "stale_days") != 14 {
		t.Fatalf("bob got alice's settings: %v", bo["currency"])
	}
	a.expect(400, a.alice, "PUT", "/api/v1/settings", map[string]any{"currency": "euro"})
	// new deals take the person's currency
	d := a.ok(a.alice, "POST", "/api/v1/deals", map[string]any{"title": "x"})
	if d["currency"] != "BAM" {
		t.Fatalf("currency: %v", d["currency"])
	}
}

func TestLinkScopes(t *testing.T) {
	type c struct {
		method, path string
		scopes       []string
		want         bool
	}
	for _, tc := range []c{
		{"GET", "/api/v1/auth/me", nil, true},
		{"GET", "/api/v1/deals", []string{"crm:read"}, true},
		{"GET", "/api/v1/deals/12", []string{"crm:read"}, true},
		{"GET", "/api/v1/lookup", []string{"crm:read"}, true},
		{"GET", "/api/v1/day/2026-11-07", []string{"crm:read"}, true},
		{"GET", "/api/v1/deals", []string{"crm:write"}, false},
		{"POST", "/api/v1/deals", []string{"crm:read"}, false},
		{"POST", "/api/v1/deals", []string{"crm:write"}, true},
		{"POST", "/api/v1/deals/3/activities", []string{"crm:write"}, true},
		{"POST", "/api/v1/deals/3/move", []string{"crm:write"}, false},
		{"POST", "/api/v1/deals/3/make", []string{"crm:read", "crm:write"}, false},
		{"DELETE", "/api/v1/deals/3", []string{"crm:write"}, false},
		{"GET", "/api/v1/settings", []string{"crm:read", "crm:write"}, false},
		{"GET", "/api/v1/export.json", []string{"crm:read"}, false},
	} {
		r, _ := http.NewRequest(tc.method, tc.path, nil)
		if got := linkAllows(tc.scopes, r); got != tc.want {
			t.Errorf("%s %s %v: got %v", tc.method, tc.path, tc.scopes, got)
		}
	}
	for _, o := range linkOffers {
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9_.:-]{0,60}$`).MatchString(o.Scope) {
			t.Errorf("bad scope %q", o.Scope)
		}
	}
}

func TestSourceRefNeverTwice(t *testing.T) {
	a := newApp(t, nil)
	first := a.expect(201, a.alice, "POST", "/api/v1/deals", map[string]any{"title": "From a form", "source_ref": "forms:answer:7"})
	again := a.expect(200, a.alice, "POST", "/api/v1/deals", map[string]any{"title": "From a form", "source_ref": "forms:answer:7"})
	if id(first) != id(again) {
		t.Fatalf("the same lead was added twice")
	}
	// another person's same ref is their own deal
	b := a.expect(201, a.bob, "POST", "/api/v1/deals", map[string]any{"title": "From a form", "source_ref": "forms:answer:7"})
	if id(b) == id(first) {
		t.Fatalf("Bob got Alice's deal")
	}
}

func TestParseAmountAndMoney(t *testing.T) {
	for in, want := range map[string]float64{"€2,000 – €5,000": 2000, "5.000 €": 5000, "about 3000": 3000, "12,50": 12.5, "1.250,75 KM": 1250.75, "Not sure yet": 0, "": 0} {
		if got := parseAmount(in); got != want {
			t.Errorf("parseAmount(%q) = %v, want %v", in, got, want)
		}
	}
	if fmtMoney(450000, "EUR") != "4,500 EUR" || fmtMoney(123456789, "BAM") != "1,234,567.89 BAM" || fmtMoney(5000, "EUR") != "50 EUR" {
		t.Errorf("fmtMoney: %s %s", fmtMoney(450000, "EUR"), fmtMoney(123456789, "BAM"))
	}
}

func TestDemoSeeds(t *testing.T) {
	a := newApp(t, nil)
	if err := a.s.seedDemo(context.Background(), a.ids["Alice"]); err != nil {
		t.Fatal(err)
	}
	l := a.ok(a.alice, "GET", "/api/v1/deals?limit=500", nil)
	if num(l, "total") < 30 {
		t.Fatalf("demo deals: %v", l["total"])
	}
	today := a.ok(a.alice, "GET", "/api/v1/today", nil)
	lists := today["lists"].(map[string]any)
	if len(lists["overdue"].([]any)) == 0 || float64(len(lists["won"].([]any)))+num(today["summary"].(map[string]any), "won_year") == 0 {
		t.Fatalf("today: %v", lists)
	}
	rep := a.ok(a.alice, "GET", "/api/v1/reports", nil)
	if len(rep["months"].([]any)) != 12 || len(rep["funnel"].([]any)) != 4 {
		t.Fatalf("reports: %v", rep)
	}
	if l := a.list(a.alice, "/api/v1/people"); len(l) < 10 {
		t.Fatalf("people: %d", len(l))
	}
	a.ok(a.alice, "GET", "/api/v1/day/"+today["date"].(string), nil)
}
