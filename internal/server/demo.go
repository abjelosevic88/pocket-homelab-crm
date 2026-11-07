package server

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

// ---- demo data (DEMO_DATA=true) ----
//
// A small web studio's sales after eight months: leads coming in from
// referrals and the website form, proposals out, a few deals won (one
// with a project and an invoice), a few lost with their reasons — enough
// to try everything and for the screenshots.

type demoPerson struct{ name, email, phone, company string }

var demoPeople = []demoPerson{
	{"Ana Marić", "ana@pekara-maric.ba", "+387 61 224 118", "Pekara Marić"}, {"Emir Hodžić", "emir@hodzic-gradnja.ba", "+387 62 410 003", "Hodžić Gradnja"},
	{"Lejla Begić", "lejla@studioatlas.ba", "", "Studio Atlas"}, {"Marko Jurić", "marko@juric-transport.hr", "+385 91 552 7710", "Jurić Transport"},
	{"Sara Kovač", "sara@kovac-law.hr", "+385 98 120 448", "Kovač & partneri"}, {"Daniel Weber", "d.weber@bergholz.de", "+49 171 220 9184", "Bergholz GmbH"},
	{"Amra Delić", "amra@delic-optika.ba", "", "Optika Delić"}, {"Nikola Petrović", "nikola@vinarija-petrovic.rs", "+381 63 771 204", "Vinarija Petrović"},
	{"Maja Novak", "maja@yogaprostor.si", "", "Yoga Prostor"}, {"Tarik Mujić", "tarik@mujic-dent.ba", "+387 61 909 312", "Ordinacija Mujić"},
	{"Jonas Lindqvist", "jonas@nordtrail.se", "+46 70 331 88 21", "Nordtrail"}, {"Selma Hadžić", "selma.hadzic@gmail.com", "", ""},
	{"Petra Horvat", "petra@horvat-design.hr", "", "Horvat Design"}, {"Adnan Kurtović", "adnan@kurtovic-auto.ba", "+387 62 330 551", "Auto Kurtović"},
	{"Nina Vuković", "nina@vukovic-events.ba", "+387 65 118 004", "Vuković Events"}, {"Luka Babić", "luka@babic-catering.hr", "", "Babić Catering"},
	{"Mia Schneider", "mia@schneider-coaching.de", "", "Schneider Coaching"}, {"Haris Omerović", "haris@omerovic-it.ba", "", "Omerović IT"},
	{"Ivana Radić", "ivana@radic-hotels.me", "+382 67 440 120", "Radić Hotels"}, {"Kenan Selimović", "kenan@bosnaeko.ba", "", "BosnaEko"},
}

var demoWork = []struct {
	title, desc string
	value       int64 // euros
}{
	{"Online shop for cakes", "Order birthday cakes online, pick a day, pay when collecting. About 40 products with photos.", 4200},
	{"Website redesign", "The old site is from 2014 and does not work on phones. Calm and trustworthy, two languages.", 3600},
	{"Delivery notes app", "Drivers take a photo and a signature on the phone instead of paper; the office sees it at once.", 12800},
	{"Wine club and shop", "Sell bottles directly, with a club that sends six bottles every quarter.", 8900},
	{"Class schedule and sign-ups", "A schedule people sign up for, 14 places per class and a waiting list.", 1400},
	{"Patient booking page", "Patients ask for a check-up time online; simple enough for older people.", 3100},
	{"Offline trail maps app", "An app with offline maps of the guided routes. Start with a two-week study?", 18500},
	{"Booking site for a guest house", "Four rooms; own booking with a calendar of free days instead of paying commission.", 6400},
	{"Faster site and service prices", "Slow site, people want prices and to book a slot.", 2700},
	{"Wedding portfolio", "Galleries and a form for couples to describe their day.", 2900},
	{"Lunch ordering for companies", "A weekly menu; companies order by 10 am.", 16200},
	{"Maintenance and hosting, a year", "Updates, backups and small changes every month.", 2400},
	{"Brand and logo", "A new name is coming; logo, colours, business cards.", 3800},
	{"Internal tools audit", "Two days looking at their spreadsheets and what to build first.", 1600},
}

var demoSteps = []string{"Call to go through the brief", "Send the proposal", "Follow up on the proposal", "Meet at their office", "Send the revised offer",
	"Ask about the budget", "Send examples of similar work", "Check if they read the contract", "Book the kick-off", "Send a short summary of the call"}

var demoNotes = []struct{ kind, title, body string }{
	{"call", "Call with them", "They want it before the season starts. Budget is fine if we split it into two parts."},
	{"meeting", "Met at their office", "Showed three examples. They liked the simple one. Worried about who adds the products later."},
	{"email", "Sent the proposal", "Two options: the full shop, or a first version with 15 products."},
	{"note", "Heard from a friend of theirs", "They also talked to another studio, but price was not the main thing."},
	{"call", "Short call", "Not ready this month; asked to call back in two weeks."},
	{"email", "They answered", "Happy with the plan, want to start after the 15th. Asked for the invoice in two parts."},
	{"note", "Idea", "Could add the newsletter later, as a second project."},
}

func demoTime(rng *rand.Rand, daysAgo int) string {
	t := time.Now().AddDate(0, 0, -daysAgo)
	t = time.Date(t.Year(), t.Month(), t.Day(), 8+rng.Intn(10), rng.Intn(60), rng.Intn(60), 0, time.Local)
	if t.After(time.Now()) {
		t = time.Now().Add(-time.Duration(30+rng.Intn(200)) * time.Minute)
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func (s *Server) seedDemo(ctx context.Context, uid int64) error {
	ctx = context.WithoutCancel(ctx)
	if _, total, _ := s.store.ListDeals(ctx, uid, store.DealFilter{Limit: 1}); total > 0 {
		return nil
	}
	stages, err := s.store.ListStages(ctx, uid)
	if err != nil {
		return err
	}
	var open []*store.Stage
	var won, lost *store.Stage
	for _, st := range stages {
		switch st.Kind {
		case store.StatusOpen:
			open = append(open, st)
		case store.StatusWon:
			won = st
		case store.StatusLost:
			lost = st
		}
	}
	rng := rand.New(rand.NewSource(42))
	reasons := store.DefaultLostReasons
	sources := []string{"referral", "referral", "website", "form", "email", "event", "repeat", "social"}
	t := today()
	for i := 0; i < 38; i++ {
		p := demoPeople[i%len(demoPeople)]
		wk := demoWork[(i*5+3)%len(demoWork)]
		age := 4 + rng.Intn(235)
		if i < 6 {
			age = 1 + rng.Intn(10)
		}
		d := &store.Deal{UserID: uid, Title: wk.title, Value: (wk.value + int64(rng.Intn(9))*100) * 100, Currency: "EUR", Probability: -1,
			Source: sources[rng.Intn(len(sources))], ContactName: p.name, ContactEmail: p.email, ContactPhone: p.phone, Company: p.company,
			Description: wk.desc, Tags: []string{}, CreatedAt: demoTime(rng, age)}
		if i%7 == 2 {
			d.Currency = "BAM"
			d.Value = d.Value * 2
		}
		if rng.Intn(4) == 0 {
			d.Tags = append(d.Tags, []string{"website", "app", "shop", "retainer"}[rng.Intn(4)])
		}
		if i%9 == 0 {
			d.Starred = true
		}
		// where it is now: the older, the more likely closed
		var stage *store.Stage
		switch r := rng.Intn(100); {
		case age > 60 && r < 40:
			stage = won
		case age > 45 && r < 62:
			stage = lost
			d.LostReason = reasons[rng.Intn(len(reasons))]
		default:
			stage = open[rng.Intn(len(open))]
		}
		if i == 6 || i == 11 || i == 17 {
			// a few won lately, for this month's numbers
			stage, age = won, 18+i
			d.CreatedAt, d.LostReason = demoTime(rng, age), ""
		}
		d.StageID = stage.ID
		if stage.Kind == store.StatusOpen {
			switch rng.Intn(5) {
			case 0:
				// overdue
				d.NextAction, d.NextDue = demoSteps[rng.Intn(len(demoSteps))], store.AddDays(t, -(1+rng.Intn(5)))
			case 1, 2:
				d.NextAction, d.NextDue = demoSteps[rng.Intn(len(demoSteps))], store.AddDays(t, rng.Intn(9))
			case 3:
				if i%2 == 0 {
					d.NextAction = demoSteps[rng.Intn(len(demoSteps))]
				}
			}
			if rng.Intn(3) > 0 {
				d.ExpectedClose = store.AddDays(t, 7+rng.Intn(70))
			}
		}
		if err := s.store.CreateDeal(ctx, d, &store.Activity{Kind: "created", Title: "Deal added", At: d.CreatedAt}); err != nil {
			return err
		}
		// a few lines on the timeline, between then and now
		n := 1 + rng.Intn(4)
		last := d.CreatedAt
		for k := 0; k < n; k++ {
			nt := demoNotes[rng.Intn(len(demoNotes))]
			at := demoTime(rng, max(age-((k+1)*age/(n+1)), 0))
			if at > last {
				last = at
			}
			_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, DealID: d.ID, Kind: nt.kind, Title: nt.title, Body: nt.body, At: at})
		}
		if stage.Kind != store.StatusOpen {
			closed := demoTime(rng, max(age-age*3/4, 1))
			if i == 6 || i == 11 || i == 17 {
				closed = demoTime(rng, 1+i%5)
			}
			if closed < last {
				closed = last
			}
			kind, title := "won", "Won 🎉"
			if stage.Kind == store.StatusLost {
				kind, title = "lost", "Lost"
			}
			_ = s.store.AddActivity(ctx, &store.Activity{UserID: uid, DealID: d.ID, Kind: kind, Title: title, Body: d.LostReason, At: closed})
			_, _ = s.store.DB.ExecContext(ctx, `UPDATE deals SET closed_at = ?, stage_changed_at = ? WHERE id = ?`, closed, closed, d.ID)
			if stage.Kind == store.StatusWon && rng.Intn(2) == 0 {
				_ = s.store.AddMade(ctx, uid, &store.Made{DealID: d.ID, Target: "project", App: "projects", Ref: fmt.Sprint(100 + i), Title: d.Company + " — " + d.Title})
				_ = s.store.AddMade(ctx, uid, &store.Made{DealID: d.ID, Target: "invoice", App: "invoicing", Ref: fmt.Sprint(200 + i), Title: fmt.Sprintf("Invoice 2026-%03d", 10+i), State: []string{"paid", "sent", "draft"}[rng.Intn(3)]})
			}
		} else {
			stageAt := demoTime(rng, max(age/3, 1))
			_, _ = s.store.DB.ExecContext(ctx, `UPDATE deals SET stage_changed_at = ? WHERE id = ?`, stageAt, d.ID)
			if rng.Intn(3) == 0 {
				_ = s.store.AddMade(ctx, uid, &store.Made{DealID: d.ID, Target: "event", App: "calendar", Ref: fmt.Sprint(300 + i), Title: "Meeting with " + d.ContactName})
			}
		}
	}
	s.log.Info("demo data seeded", "user", uid)
	return nil
}
