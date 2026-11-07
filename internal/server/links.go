package server

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"

	core "github.com/abjelosevic88/pocket-homelab-core/sdk"
	"github.com/abjelosevic88/pocket-homelab-crm/internal/store"
)

// Pocket Core app links (docs/links.md in pocket-homelab-core), CRM as
// the target: another Pocket app the person allowed in Core may act here for
// them with a pcx_ token, only within these scopes. Every route not listed
// in linkRoutes stays closed to it (403).

var linkOffers = []core.Offer{
	{Scope: "crm:read", Name: "See your deals", Description: "Read your deals (who, what they are worth, the stage, the next step and the timeline), the pipeline and the totals — for a contact's page, your journal, search, dashboards and Pocket AI."},
	{Scope: "crm:write", Name: "Add leads and notes to your deals", Description: "Add a new lead for you (an answer to a form, an email), and add a line to a deal's timeline (\"called Ana\", \"sent the offer\"). It may change only the deals that app added itself."},
}

type linkRoute struct {
	methods string
	path    *regexp.Regexp
	scope   string // "" = any linked app
}

var linkRoutes = []linkRoute{
	{"GET", regexp.MustCompile(`^/api/v1/auth/me$`), ""},
	{"GET", regexp.MustCompile(`^/api/v1/(deals|stages|summary|search|people|lookup|reports|tags|today|day/\d{4}-\d{2}-\d{2})$`), "crm:read"},
	{"GET", regexp.MustCompile(`^/api/v1/deals/\d+(/activities)?$`), "crm:read"},
	{"POST", regexp.MustCompile(`^/api/v1/deals$`), "crm:write"},
	{"PUT", regexp.MustCompile(`^/api/v1/deals/\d+$`), "crm:write"},
	{"POST", regexp.MustCompile(`^/api/v1/deals/\d+/activities$`), "crm:write"},
}

var errLinkScope = errors.New("outside the link's scope")

// linkAllows reports whether a link token with these scopes may make this request.
func linkAllows(scopes []string, r *http.Request) bool {
	for _, lr := range linkRoutes {
		if !strings.Contains(" "+lr.methods+" ", " "+r.Method+" ") || !lr.path.MatchString(r.URL.Path) {
			continue
		}
		if lr.scope == "" {
			return true
		}
		for _, sc := range scopes {
			if sc == lr.scope {
				return true
			}
		}
	}
	return false
}

// linkUser validates a pcx_ token, checks the route against its scopes and
// maps it to the person it acts for. The second value is the acting app's slug.
func (s *Server) linkUser(ctx context.Context, tok string, r *http.Request) (*store.User, string, error) {
	in, err := s.core.Introspect(ctx, tok)
	if err != nil {
		return nil, "", err
	}
	if !in.Active || !in.Delegated() || in.Audience != "crm" || in.User == nil || in.User.Email == "" || in.App == nil || in.App.Slug == "" {
		return nil, "", http.ErrNoCookie
	}
	if !linkAllows(strings.Fields(in.Scope), r) {
		return nil, "", errLinkScope
	}
	u, err := s.coreIdentityUser(ctx, in)
	return u, in.App.Slug, err
}

const linkAppKey ctxKey = 2

// linkApp is the acting app's slug when the request came in on a link token.
func linkApp(ctx context.Context) string {
	a, _ := ctx.Value(linkAppKey).(string)
	return a
}
