package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

// ---- Pocket AI on a deal ----
//
// Three helps, each on one deal and only for its owner: what to do next,
// where the deal stands in a few lines, and a follow-up email to send.
// The AI sees the deal and its timeline, nothing else.

func (s *Server) dealBrief(ctx context.Context, d *store.Deal) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Deal: %s\nStage: %s (%s)\n", d.Title, d.StageName, d.Status)
	if who := d.Who(); who != "" {
		fmt.Fprintf(&b, "With: %s\n", who)
	}
	if d.Value > 0 {
		fmt.Fprintf(&b, "Worth: %s\n", fmtMoney(d.Value, d.Currency))
	}
	if d.ExpectedClose != "" {
		fmt.Fprintf(&b, "Hoped to close by: %s\n", d.ExpectedClose)
	}
	if d.NextAction != "" {
		fmt.Fprintf(&b, "Next step planned: %s (due %s)\n", d.NextAction, firstNonEmpty(d.NextDue, "no day"))
	}
	fmt.Fprintf(&b, "Added: %s; last thing that happened: %s; today: %s\n", day10(d.CreatedAt), day10(d.LastActivityAt), today())
	if d.Description != "" {
		fmt.Fprintf(&b, "\nAbout the deal:\n%s\n", clip(d.Description, 3000))
	}
	acts, _ := s.store.ListActivities(ctx, d.UserID, d.ID, 40)
	if len(acts) > 0 {
		b.WriteString("\nTimeline (newest first):\n")
		for _, a := range acts {
			fmt.Fprintf(&b, "- %s [%s] %s", day10(a.At), a.Kind, a.Title)
			if a.Body != "" {
				fmt.Fprintf(&b, ": %s", strings.ReplaceAll(clip(a.Body, 500), "\n", " "))
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

const aiSystem = `You help a freelancer or a small business owner with their sales. Write plainly and briefly, like a sensible colleague. No jargon, no hype, no emoji. Use the language the deal is written in.`

func (s *Server) handleAIDeal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	d, ok := s.ownDeal(w, r)
	if !ok {
		return
	}
	st := s.settingsOf(ctx, d.UserID)
	if !st.AIHelp {
		writeErr(w, http.StatusBadRequest, "AI help is switched off (Settings → AI)")
		return
	}
	cl, _, why := s.aiFor(ctx, d.UserID, st)
	if cl == nil {
		writeErr(w, http.StatusServiceUnavailable, firstNonEmpty(why, "no AI is set up (Settings → AI)"))
		return
	}
	var in struct {
		Want string `json:"want"` // next | summary | email
		Hint string `json:"hint"` // what the email should say
	}
	if err := decode(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	select {
	case s.aiSem <- struct{}{}:
		defer func() { <-s.aiSem }()
	case <-time.After(20 * time.Second):
		writeErr(w, http.StatusServiceUnavailable, "the AI is busy, try again in a moment")
		return
	}
	brief := s.dealBrief(ctx, d)
	me := userFrom(ctx)
	switch in.Want {
	case "next":
		var out struct {
			Step string `json:"step"`
			Due  string `json:"due"`
			Why  string `json:"why"`
		}
		err := cl.JSON(ctx, aiSystem+` Answer as JSON {"step": "...", "due": "YYYY-MM-DD", "why": "..."}. step is ONE concrete next step (at most 12 words, starting with a verb). due is a sensible day for it, not in the past. why is one short sentence.`, brief, &out)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		if !validDate(out.Due) || out.Due < today() {
			out.Due = store.AddDays(today(), max(st.ActionDays, 1))
		}
		writeJSON(w, http.StatusOK, map[string]any{"step": clip(strings.TrimSpace(out.Step), 300), "due": out.Due, "why": clip(out.Why, 400)})
	case "summary":
		txt, err := cl.Complete(ctx, aiSystem, "Sum up where this deal stands in three to five short lines: what they want, what happened, what is open, and the risk if any.\n\n"+brief, false)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"text": strings.TrimSpace(txt)})
	case "email":
		var out struct {
			Subject string `json:"subject"`
			Body    string `json:"body"`
		}
		sign := ""
		if me != nil {
			sign = firstNonEmpty(me.Name, "")
		}
		prompt := brief + "\nWrite a short follow-up email to " + firstNonEmpty(d.ContactName, "them") + "."
		if h := strings.TrimSpace(in.Hint); h != "" {
			prompt += " It should say: " + clip(h, 1000)
		}
		if sign != "" {
			prompt += " Sign it " + sign + "."
		}
		err := cl.JSON(ctx, aiSystem+` Answer as JSON {"subject": "...", "body": "..."}. The body is plain text with blank lines between paragraphs, friendly and to the point, at most 120 words.`, prompt, &out)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"subject": clip(strings.TrimSpace(out.Subject), 200), "body": strings.TrimSpace(out.Body)})
	default:
		writeErr(w, http.StatusBadRequest, "ask for next, summary or email")
	}
}
