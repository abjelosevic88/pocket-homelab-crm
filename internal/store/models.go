package store

import (
	"encoding/json"
	"strings"
)

// User is an account on this box.
type User struct {
	ID           int64  `json:"id"`
	Email        string `json:"email"`
	Name         string `json:"name"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"` // admin | member
	CreatedAt    string `json:"created_at"`
}

// APIToken is a personal access token (hash stored, plaintext shown once).
type APIToken struct {
	ID         int64   `json:"id"`
	UserID     int64   `json:"user_id"`
	Name       string  `json:"name"`
	Prefix     string  `json:"prefix"`
	LastUsedAt *string `json:"last_used_at"`
	CreatedAt  string  `json:"created_at"`
}

// Stage kinds and deal statuses.
const (
	StatusOpen = "open"
	StatusWon  = "won"
	StatusLost = "lost"
)

// Stage is one step of a person's pipeline.
type Stage struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"-"`
	Name        string `json:"name"`
	Kind        string `json:"kind"` // open | won | lost
	Position    int    `json:"position"`
	Probability int    `json:"probability"`
	Color       string `json:"color"`
	CreatedAt   string `json:"created_at"`

	// Filled for lists.
	Deals int   `json:"deals"`
	Value int64 `json:"value"`
}

// DefaultStages is the pipeline a person starts with.
var DefaultStages = []Stage{
	{Name: "New lead", Kind: StatusOpen, Probability: 10, Color: "#868e96"},
	{Name: "First talk", Kind: StatusOpen, Probability: 25, Color: "#1c7ed6"},
	{Name: "Proposal sent", Kind: StatusOpen, Probability: 50, Color: "#7048e8"},
	{Name: "Negotiating", Kind: StatusOpen, Probability: 75, Color: "#f08c00"},
	{Name: "Won", Kind: StatusWon, Probability: 100, Color: "#2f9e44"},
	{Name: "Lost", Kind: StatusLost, Probability: 0, Color: "#e03131"},
}

// Sources are where a deal can come from.
var Sources = []string{"referral", "website", "form", "email", "event", "social", "repeat", "cold", "other"}

// Deal is possible work for someone.
type Deal struct {
	ID             int64    `json:"id"`
	UserID         int64    `json:"-"`
	Title          string   `json:"title"`
	StageID        int64    `json:"stage_id"`
	Status         string   `json:"status"`
	Value          int64    `json:"value"` // cents
	Currency       string   `json:"currency"`
	Probability    int      `json:"probability"` // -1 = the stage's
	ExpectedClose  string   `json:"expected_close"`
	Source         string   `json:"source"`
	ContactName    string   `json:"contact_name"`
	ContactEmail   string   `json:"contact_email"`
	ContactPhone   string   `json:"contact_phone"`
	ContactRef     string   `json:"contact_ref"`
	Company        string   `json:"company"`
	CompanyRef     string   `json:"company_ref"`
	Description    string   `json:"description"`
	NextAction     string   `json:"next_action"`
	NextDue        string   `json:"next_due"`
	NextTaskRef    string   `json:"next_task_ref"`
	Tags           []string `json:"tags"`
	Starred        bool     `json:"starred"`
	Position       float64  `json:"position"`
	LostReason     string   `json:"lost_reason"`
	OutcomeNote    string   `json:"outcome_note"`
	ClosedAt       string   `json:"closed_at"`
	StageChangedAt string   `json:"stage_changed_at"`
	LastActivityAt string   `json:"last_activity_at"`
	SourceApp      string   `json:"source_app"`
	SourceRef      string   `json:"source_ref"`
	ClientKey      string   `json:"client_key,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`

	// Filled for lists and pages.
	StageName   string  `json:"stage_name,omitempty"`
	StageColor  string  `json:"stage_color,omitempty"`
	Amount      float64 `json:"amount"`   // value in the currency's units (4500.00)
	Chance      int     `json:"chance"`   // the probability in effect
	Weighted    int64   `json:"weighted"` // value × chance
	Overdue     bool    `json:"overdue"`
	Stale       bool    `json:"stale"` // nothing happened for a while
	MadeCount   int     `json:"made_count"`
	Activities  int     `json:"activities"`
	DaysInStage int     `json:"days_in_stage"`
}

// Who is how to call the other side of a deal.
func (d *Deal) Who() string {
	switch {
	case d.ContactName != "" && d.Company != "":
		return d.ContactName + " (" + d.Company + ")"
	case d.ContactName != "":
		return d.ContactName
	case d.Company != "":
		return d.Company
	}
	return d.ContactEmail
}

// Activity kinds a person can add by hand; the rest are written by the app.
var ActivityKinds = []string{"note", "call", "meeting", "email", "step"}

// Activity is one thing that happened on a deal.
type Activity struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"-"`
	DealID    int64  `json:"deal_id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	At        string `json:"at"`
	App       string `json:"app"`
	URL       string `json:"url"`
	SourceApp string `json:"source_app"`
	CreatedAt string `json:"created_at"`
}

// Made is something a deal has in another Pocket app.
type Made struct {
	ID        int64  `json:"id"`
	DealID    int64  `json:"deal_id"`
	Target    string `json:"target"`
	App       string `json:"app"`
	Ref       string `json:"ref"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
	CheckedAt string `json:"checked_at"`
	CreatedAt string `json:"created_at"`
}

// Conn is one person's connection to a sibling Pocket app.
type Conn struct {
	Mode      string `json:"mode,omitempty"`
	URL       string `json:"url,omitempty"`
	Token     string `json:"token,omitempty"`
	PublicURL string `json:"public_url,omitempty"`
}

// Settings are the box-wide values plus one person's preferences and their
// own connections.
type Settings struct {
	Title         string `json:"title"`
	SetupComplete bool   `json:"setup_complete"`

	DateFormat string `json:"date_format"`
	TimeFormat string `json:"time_format"`
	WeekStart  int    `json:"week_start"` // 1 = Monday, 7 = Sunday

	// Deals.
	Currency    string   `json:"currency"`     // new deals start in this currency
	StaleDays   int      `json:"stale_days"`   // an open deal with nothing for this many days needs a look
	ActionDays  int      `json:"action_days"`  // a new next step is due this many days later
	LostReasons []string `json:"lost_reasons"` // offered when a deal is lost

	// The other apps.
	TaskMirror     bool   `json:"task_mirror"`     // next steps also go to Pocket Tasks
	MeetingMinutes int    `json:"meeting_minutes"` // how long a new meeting is
	ProjectTeam    int64  `json:"project_team"`    // Pocket Projects team for won deals (0 = Projects' default)
	MailFrom       string `json:"mail_from"`       // which Pocket Mail account emails leave from ("" = its default)
	WonProject     bool   `json:"won_project"`     // "Won" offers these, ticked
	WonInvoice     bool   `json:"won_invoice"`
	WonTask        bool   `json:"won_task"`
	AIHelp         bool   `json:"ai_help"`

	// Notifications.
	NotifyDigest bool   `json:"notify_digest"` // morning: next steps due today, deals gone quiet
	NotifyHour   int    `json:"notify_hour"`
	NotifyURL    string `json:"notify_url"`
	NotifyFormat string `json:"notify_format"` // apprise | ntfy | json

	// Each sibling Pocket app, by slug.
	Conns map[string]Conn `json:"conns"`

	// AI: Pocket AI through Core, or an OpenAI-compatible API with the
	// person's own key.
	AIProvider    string `json:"ai_provider"` // auto | pocket | openai
	AIPocketModel string `json:"ai_pocket_model"`
	OpenAIKey     string `json:"openai_api_key"`
	OpenAIModel   string `json:"openai_model"`
	OpenAIBaseURL string `json:"openai_base_url"`
}

// DefaultLostReasons are offered until a person changes them.
var DefaultLostReasons = []string{"Price too high", "Went with someone else", "No budget", "Bad timing", "No answer", "Not a good fit"}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func cleanTags(in []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, t := range in {
		t = strings.TrimSpace(strings.TrimPrefix(t, "#"))
		k := strings.ToLower(t)
		if t == "" || seen[k] || len(out) >= 20 {
			continue
		}
		seen[k] = true
		out = append(out, t)
	}
	return out
}
