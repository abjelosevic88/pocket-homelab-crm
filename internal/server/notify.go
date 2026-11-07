package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-crm/internal/notify"
	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

// pushes runs every minute; once a day, at each person's own hour, it
// says which next steps are due and which deals went quiet (when they want
// that), and tells the box (crm.steps_due) for Pocket Flows.
func (s *Server) pushes(ctx context.Context) {
	ids, err := s.store.SettingsUsers(ctx)
	if err != nil {
		return
	}
	now := time.Now()
	for _, uid := range ids {
		st, err := s.store.GetSettings(ctx, uid)
		if err != nil || !st.NotifyDigest || now.Hour() != st.NotifyHour || now.Minute() > 10 {
			continue
		}
		if !s.once(ctx, fmt.Sprintf("push:%d:%s", uid, store.Today())) {
			continue
		}
		s.dailyCheck(ctx, uid, st, now)
	}
}

// once is true the first time it is asked for a key.
func (s *Server) once(ctx context.Context, key string) bool {
	if _, _, done := s.store.CacheGet(ctx, key); done {
		return false
	}
	_ = s.store.CachePut(ctx, key, "1")
	return true
}

// dailyCheck builds and sends one person's morning message: the next steps
// due today and overdue, and the deals nobody touched for a while.
func (s *Server) dailyCheck(ctx context.Context, uid int64, st store.Settings, now time.Time) (string, bool) {
	all, err := s.store.AllDeals(ctx, uid)
	if err != nil {
		return "", false
	}
	decorate(st, all...)
	t := now.Format("2006-01-02")
	var due, late, quiet []*store.Deal
	for _, d := range all {
		if d.Status != store.StatusOpen {
			continue
		}
		switch {
		case d.NextDue != "" && d.NextDue < t:
			late = append(late, d)
		case d.NextDue == t:
			due = append(due, d)
		}
		if d.Stale {
			quiet = append(quiet, d)
		}
	}
	if len(due)+len(late)+len(quiet) == 0 {
		return "", false
	}
	var lines []string
	step := func(d *store.Deal) string {
		return fmt.Sprintf("%s (%s)", firstNonEmpty(d.NextAction, "next step"), d.Title)
	}
	if len(due) > 0 {
		var parts []string
		for _, d := range due[:min(len(due), 4)] {
			parts = append(parts, step(d))
		}
		lines = append(lines, fmt.Sprintf("Today: %s.", strings.Join(parts, "; ")))
	}
	if len(late) > 0 {
		lines = append(lines, fmt.Sprintf("%d %s overdue.", len(late), plural(len(late), "step is", "steps are")))
	}
	if len(quiet) > 0 {
		lines = append(lines, fmt.Sprintf("%d %s with no next step for %d days or more.", len(quiet), plural(len(quiet), "deal is quiet", "deals are quiet"), st.StaleDays))
	}
	body := strings.Join(lines, "\n")
	s.publish(ctx, uid, "crm.steps_due", "steps:"+t, map[string]any{"date": t, "due": len(due), "overdue": len(late), "quiet": len(quiet), "text": body})
	link := strings.TrimRight(s.cfg.BaseURL, "/") + "/"
	if !s.push(ctx, uid, st, "Your deals today", body, link) {
		return body, false
	}
	return body, true
}

// push sends one message to a person's own notification address.
func (s *Server) push(ctx context.Context, uid int64, st store.Settings, title, body, link string) bool {
	if st.NotifyURL == "" {
		return false
	}
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: title, Body: body, Priority: "info", Tags: []string{"handshake"}, Link: link}); err != nil {
		s.log.Warn("notify failed", "user", uid, "err", err)
		return false
	}
	return true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	st, _ := s.store.GetSettings(ctx, actorID(ctx))
	n := notify.New(st.NotifyURL, st.NotifyFormat)
	if err := n.Send(ctx, notify.Message{Title: "Pocket CRM", Body: notifyTestBody, Priority: "info", Tags: []string{"handshake"}}); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

const notifyTestBody = "Notifications work. Each morning, if you want it, a note says which next steps on your deals are due and which deals went quiet."
