package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
)

// ---- deals ----

const dealCols = `d.id, d.user_id, d.title, d.stage_id, d.status, d.value, d.currency, d.probability, d.expected_close, d.source,
	d.contact_name, d.contact_email, d.contact_phone, d.contact_ref, d.company, d.company_ref, d.description,
	d.next_action, d.next_due, d.next_task_ref, d.tags, d.starred, d.position, d.lost_reason, d.outcome_note, d.closed_at,
	d.stage_changed_at, d.last_activity_at, d.source_app, d.source_ref, d.client_key, d.created_at, d.updated_at,
	COALESCE(s.name, ''), COALESCE(s.color, ''), COALESCE(s.probability, 0),
	(SELECT COUNT(1) FROM made m WHERE m.deal_id = d.id AND m.error = ''),
	(SELECT COUNT(1) FROM activities a WHERE a.deal_id = d.id)`

const dealFrom = ` FROM deals d LEFT JOIN stages s ON s.id = d.stage_id `

func scanDeal(sc interface{ Scan(...any) error }) (*Deal, error) {
	var d Deal
	var tags string
	var starred int
	var stageProb int
	if err := sc.Scan(&d.ID, &d.UserID, &d.Title, &d.StageID, &d.Status, &d.Value, &d.Currency, &d.Probability, &d.ExpectedClose, &d.Source,
		&d.ContactName, &d.ContactEmail, &d.ContactPhone, &d.ContactRef, &d.Company, &d.CompanyRef, &d.Description,
		&d.NextAction, &d.NextDue, &d.NextTaskRef, &tags, &starred, &d.Position, &d.LostReason, &d.OutcomeNote, &d.ClosedAt,
		&d.StageChangedAt, &d.LastActivityAt, &d.SourceApp, &d.SourceRef, &d.ClientKey, &d.CreatedAt, &d.UpdatedAt,
		&d.StageName, &d.StageColor, &stageProb, &d.MadeCount, &d.Activities); err != nil {
		return nil, err
	}
	d.Starred = starred == 1
	_ = json.Unmarshal([]byte(tags), &d.Tags)
	if d.Tags == nil {
		d.Tags = []string{}
	}
	d.Chance = d.Probability
	if d.Chance < 0 {
		d.Chance = stageProb
	}
	switch d.Status {
	case StatusWon:
		d.Chance = 100
	case StatusLost:
		d.Chance = 0
	}
	d.Weighted = d.Value * int64(d.Chance) / 100
	d.Amount = float64(d.Value) / 100
	return &d, nil
}

// DealFilter narrows a list of deals.
type DealFilter struct {
	Status  string // open | won | lost | closed | "" = all
	StageID int64
	Search  string
	Company string
	Email   string
	Due     string // overdue | today | week | none | any
	Today   string // YYYY-MM-DD the Due filter counts from
	Starred bool
	Tag     string
	Source  string
	Sort    string // stage | due | value | updated | created | close
	Limit   int
	Offset  int
}

func (f DealFilter) where(uid int64) (string, []any) {
	w := []string{"d.user_id = ?"}
	args := []any{uid}
	add := func(c string, a ...any) { w = append(w, c); args = append(args, a...) }
	switch f.Status {
	case StatusOpen, StatusWon, StatusLost:
		add("d.status = ?", f.Status)
	case "closed":
		add("d.status <> 'open'")
	}
	if f.StageID > 0 {
		add("d.stage_id = ?", f.StageID)
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		like := "%" + q + "%"
		add(`(d.title LIKE ? OR d.contact_name LIKE ? OR d.contact_email LIKE ? OR d.company LIKE ? OR d.description LIKE ? OR d.next_action LIKE ? OR d.tags LIKE ?)`,
			like, like, like, like, like, like, like)
	}
	if f.Company != "" {
		add("lower(d.company) = lower(?)", f.Company)
	}
	if f.Email != "" {
		add("lower(d.contact_email) = lower(?)", f.Email)
	}
	today := f.Today
	if today == "" {
		today = Today()
	}
	switch f.Due {
	case "overdue":
		add("d.next_due <> '' AND d.next_due < ?", today)
	case "today":
		add("d.next_due <> '' AND d.next_due <= ?", today)
	case "week":
		add("d.next_due <> '' AND d.next_due <= ?", AddDays(today, 7))
	case "none":
		add("d.next_due = '' AND d.next_action = ''")
	case "any":
		add("(d.next_due <> '' OR d.next_action <> '')")
	}
	if f.Starred {
		add("d.starred = 1")
	}
	if f.Tag != "" {
		add("d.tags LIKE ?", `%"`+f.Tag+`"%`)
	}
	if f.Source != "" {
		add("d.source = ?", f.Source)
	}
	return " WHERE " + strings.Join(w, " AND "), args
}

func (f DealFilter) order() string {
	switch f.Sort {
	case "due":
		return ` ORDER BY d.next_due = '', d.next_due, d.updated_at DESC`
	case "value":
		return ` ORDER BY d.value DESC, d.id DESC`
	case "created":
		return ` ORDER BY d.created_at DESC, d.id DESC`
	case "close":
		return ` ORDER BY d.closed_at DESC, d.updated_at DESC`
	case "expected":
		return ` ORDER BY d.expected_close = '', d.expected_close, d.id`
	case "stage":
		return ` ORDER BY s.position, d.position, d.id`
	}
	return ` ORDER BY d.updated_at DESC, d.id DESC`
}

// ListDeals returns a page of a person's deals and how many match.
func (s *Store) ListDeals(ctx context.Context, uid int64, f DealFilter) ([]*Deal, int, error) {
	where, args := f.where(uid)
	var total int
	if err := s.DB.QueryRowContext(ctx, `SELECT COUNT(1)`+dealFrom+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	limit := f.Limit
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+dealCols+dealFrom+where+f.order()+` LIMIT ? OFFSET ?`, append(args, limit, f.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []*Deal{}
	for rows.Next() {
		d, err := scanDeal(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

// GetDeal returns one of a person's deals.
func (s *Store) GetDeal(ctx context.Context, uid, id int64) (*Deal, error) {
	d, err := scanDeal(s.DB.QueryRowContext(ctx, `SELECT `+dealCols+dealFrom+` WHERE d.user_id = ? AND d.id = ?`, uid, id))
	return d, notFound(err)
}

// DealBySource finds the deal made from something elsewhere (forms:answer:12).
func (s *Store) DealBySource(ctx context.Context, uid int64, ref string) (*Deal, error) {
	d, err := scanDeal(s.DB.QueryRowContext(ctx, `SELECT `+dealCols+dealFrom+` WHERE d.user_id = ? AND d.source_ref = ?`, uid, ref))
	return d, notFound(err)
}

// DealByClientKey finds an imported deal.
func (s *Store) DealByClientKey(ctx context.Context, uid int64, key string) (*Deal, error) {
	d, err := scanDeal(s.DB.QueryRowContext(ctx, `SELECT `+dealCols+dealFrom+` WHERE d.user_id = ? AND d.client_key = ?`, uid, key))
	return d, notFound(err)
}

func (s *Store) stageKind(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, uid, stageID int64) (string, error) {
	var kind string
	err := q.QueryRowContext(ctx, `SELECT kind FROM stages WHERE user_id = ? AND id = ?`, uid, stageID).Scan(&kind)
	return kind, notFound(err)
}

// CreateDeal stores a new deal at the top of its stage, and its first line
// on the timeline.
func (s *Store) CreateDeal(ctx context.Context, d *Deal, first *Activity) error {
	if d.StageID == 0 {
		st, err := s.FirstStage(ctx, d.UserID, StatusOpen)
		if err != nil {
			return err
		}
		d.StageID = st.ID
	}
	kind, err := s.stageKind(ctx, s.DB, d.UserID, d.StageID)
	if err != nil {
		return err
	}
	d.Status = kind
	if d.Status != StatusOpen && d.ClosedAt == "" {
		d.ClosedAt = Now()
	}
	d.Tags = cleanTags(d.Tags)
	var minPos sql.NullFloat64
	_ = s.DB.QueryRowContext(ctx, `SELECT MIN(position) FROM deals WHERE user_id = ? AND stage_id = ?`, d.UserID, d.StageID).Scan(&minPos)
	d.Position = minPos.Float64 - 1
	now := Now()
	if d.CreatedAt == "" {
		d.CreatedAt = now
	}
	if d.LastActivityAt == "" {
		d.LastActivityAt = d.CreatedAt
	}
	if d.StageChangedAt == "" {
		d.StageChangedAt = d.CreatedAt
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO deals (user_id, title, stage_id, status, value, currency, probability, expected_close, source,
		contact_name, contact_email, contact_phone, contact_ref, company, company_ref, description, next_action, next_due, next_task_ref,
		tags, starred, position, lost_reason, outcome_note, closed_at, stage_changed_at, last_activity_at, source_app, source_ref, client_key,
		created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.UserID, d.Title, d.StageID, d.Status, d.Value, d.Currency, d.Probability, d.ExpectedClose, d.Source,
		d.ContactName, d.ContactEmail, d.ContactPhone, d.ContactRef, d.Company, d.CompanyRef, d.Description, d.NextAction, d.NextDue, d.NextTaskRef,
		toJSON(d.Tags), boolInt(d.Starred), d.Position, d.LostReason, d.OutcomeNote, d.ClosedAt, d.StageChangedAt, d.LastActivityAt,
		d.SourceApp, d.SourceRef, d.ClientKey, d.CreatedAt, now)
	if err != nil {
		_ = tx.Rollback()
		return conflict(err)
	}
	d.ID, _ = res.LastInsertId()
	if first != nil {
		first.UserID, first.DealID = d.UserID, d.ID
		if first.At == "" {
			first.At = d.CreatedAt
		}
		if err := insertActivity(ctx, tx, first); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// UpdateDeal saves a deal's fields (not its stage: MoveDeal does that).
func (s *Store) UpdateDeal(ctx context.Context, d *Deal) error {
	d.Tags = cleanTags(d.Tags)
	res, err := s.DB.ExecContext(ctx, `UPDATE deals SET title = ?, value = ?, currency = ?, probability = ?, expected_close = ?, source = ?,
		contact_name = ?, contact_email = ?, contact_phone = ?, contact_ref = ?, company = ?, company_ref = ?, description = ?,
		next_action = ?, next_due = ?, next_task_ref = ?, tags = ?, starred = ?, lost_reason = ?, outcome_note = ?, updated_at = ?
		WHERE user_id = ? AND id = ?`,
		d.Title, d.Value, d.Currency, d.Probability, d.ExpectedClose, d.Source,
		d.ContactName, d.ContactEmail, d.ContactPhone, d.ContactRef, d.Company, d.CompanyRef, d.Description,
		d.NextAction, d.NextDue, d.NextTaskRef, toJSON(d.Tags), boolInt(d.Starred), d.LostReason, d.OutcomeNote, Now(), d.UserID, d.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveDeal puts a deal in a stage (and a place in it: between the deals
// with these positions; both 0 = the top). Winning or losing closes it;
// moving back to an open stage opens it again. It returns the old stage.
func (s *Store) MoveDeal(ctx context.Context, uid, id, stageID int64, before, after *float64) (old *Stage, err error) {
	d, err := s.GetDeal(ctx, uid, id)
	if err != nil {
		return nil, err
	}
	old, _ = s.GetStage(ctx, uid, d.StageID)
	kind, err := s.stageKind(ctx, s.DB, uid, stageID)
	if err != nil {
		return nil, err
	}
	pos := d.Position
	switch {
	case before != nil && after != nil:
		pos = (*before + *after) / 2
	case before != nil:
		pos = *before + 1
	case after != nil:
		pos = *after - 1
	case stageID != d.StageID:
		var minPos sql.NullFloat64
		_ = s.DB.QueryRowContext(ctx, `SELECT MIN(position) FROM deals WHERE user_id = ? AND stage_id = ?`, uid, stageID).Scan(&minPos)
		pos = minPos.Float64 - 1
	}
	now := Now()
	changed, closed := d.StageChangedAt, d.ClosedAt
	if stageID != d.StageID {
		changed = now
	}
	switch {
	case kind == StatusOpen:
		closed = ""
	case d.Status != kind || closed == "":
		closed = now
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE deals SET stage_id = ?, status = ?, position = ?, stage_changed_at = ?, closed_at = ?, updated_at = ?,
		lost_reason = CASE WHEN ? = 'lost' THEN lost_reason ELSE '' END WHERE user_id = ? AND id = ?`,
		stageID, kind, pos, changed, closed, now, kind, uid, id)
	return old, err
}

// SetNextStep changes only a deal's next step.
func (s *Store) SetNextStep(ctx context.Context, uid, id int64, action, due, taskRef string) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE deals SET next_action = ?, next_due = ?, next_task_ref = ?, updated_at = ? WHERE user_id = ? AND id = ?`,
		action, due, taskRef, Now(), uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetContactRef links a deal to a card in Pocket Contacts.
func (s *Store) SetContactRef(ctx context.Context, uid, id int64, contactRef, companyRef string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE deals SET contact_ref = CASE WHEN ? <> '' THEN ? ELSE contact_ref END,
		company_ref = CASE WHEN ? <> '' THEN ? ELSE company_ref END WHERE user_id = ? AND id = ?`,
		contactRef, contactRef, companyRef, companyRef, uid, id)
	return err
}

// DeleteDeal removes a deal with its timeline.
func (s *Store) DeleteDeal(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM deals WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Tags lists the tags a person uses, most used first.
func (s *Store) Tags(ctx context.Context, uid int64) ([]string, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT tags FROM deals WHERE user_id = ? AND tags <> '[]'`, uid)
	if err != nil {
		return nil, err
	}
	n := map[string]int{}
	var order []string
	for rows.Next() {
		var raw string
		if rows.Scan(&raw) != nil {
			continue
		}
		var ts []string
		_ = json.Unmarshal([]byte(raw), &ts)
		for _, t := range ts {
			if n[t] == 0 {
				order = append(order, t)
			}
			n[t]++
		}
	}
	rows.Close()
	for i := 1; i < len(order); i++ {
		for j := i; j > 0 && n[order[j]] > n[order[j-1]]; j-- {
			order[j], order[j-1] = order[j-1], order[j]
		}
	}
	if order == nil {
		order = []string{}
	}
	return order, rows.Err()
}

// ---- the timeline ----

const activityCols = `id, user_id, deal_id, kind, title, body, at, app, url, source_app, created_at`

func scanActivity(sc interface{ Scan(...any) error }) (*Activity, error) {
	var a Activity
	err := sc.Scan(&a.ID, &a.UserID, &a.DealID, &a.Kind, &a.Title, &a.Body, &a.At, &a.App, &a.URL, &a.SourceApp, &a.CreatedAt)
	return &a, err
}

func insertActivity(ctx context.Context, tx *sql.Tx, a *Activity) error {
	if a.At == "" {
		a.At = Now()
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO activities (user_id, deal_id, kind, title, body, at, app, url, source_app) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.UserID, a.DealID, a.Kind, a.Title, a.Body, a.At, a.App, a.URL, a.SourceApp)
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	// what a person did (not what is planned) counts as the deal's last activity
	_, err = tx.ExecContext(ctx, `UPDATE deals SET last_activity_at = MAX(last_activity_at, ?) WHERE user_id = ? AND id = ?`, a.At, a.UserID, a.DealID)
	return err
}

// AddActivity adds a line to a deal's timeline.
func (s *Store) AddActivity(ctx context.Context, a *Activity) error {
	if _, err := s.GetDeal(ctx, a.UserID, a.DealID); err != nil {
		return err
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := insertActivity(ctx, tx, a); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// ListActivities returns a deal's timeline, newest first.
func (s *Store) ListActivities(ctx context.Context, uid, dealID int64, limit int) ([]*Activity, error) {
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+activityCols+` FROM activities WHERE user_id = ? AND deal_id = ? ORDER BY at DESC, id DESC LIMIT ?`, uid, dealID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Activity{}
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ActivitiesBetween returns a person's timeline lines in [from, to) (RFC3339).
func (s *Store) ActivitiesBetween(ctx context.Context, uid int64, from, to string, limit int) ([]*Activity, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT `+activityCols+` FROM activities WHERE user_id = ? AND at >= ? AND at < ? ORDER BY at DESC, id DESC LIMIT ?`, uid, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Activity{}
	for rows.Next() {
		a, err := scanActivity(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetActivity returns one line of a person's timeline.
func (s *Store) GetActivity(ctx context.Context, uid, id int64) (*Activity, error) {
	a, err := scanActivity(s.DB.QueryRowContext(ctx, `SELECT `+activityCols+` FROM activities WHERE user_id = ? AND id = ?`, uid, id))
	return a, notFound(err)
}

// UpdateActivity saves a line's words and time.
func (s *Store) UpdateActivity(ctx context.Context, a *Activity) error {
	res, err := s.DB.ExecContext(ctx, `UPDATE activities SET kind = ?, title = ?, body = ?, at = ? WHERE user_id = ? AND id = ?`, a.Kind, a.Title, a.Body, a.At, a.UserID, a.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteActivity removes a line.
func (s *Store) DeleteActivity(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM activities WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---- in the other apps ----

const madeCols = `id, deal_id, target, app, ref, title, url, state, error, checked_at, created_at`

// ListMade returns what a deal has in the other apps (failed tries last).
func (s *Store) ListMade(ctx context.Context, uid, dealID int64) ([]*Made, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+madeCols+` FROM made WHERE user_id = ? AND deal_id = ? ORDER BY error <> '', created_at DESC, id DESC`, uid, dealID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Made{}
	for rows.Next() {
		var m Made
		if err := rows.Scan(&m.ID, &m.DealID, &m.Target, &m.App, &m.Ref, &m.Title, &m.URL, &m.State, &m.Error, &m.CheckedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// AddMade remembers something a deal has in another app.
func (s *Store) AddMade(ctx context.Context, uid int64, m *Made) error {
	res, err := s.DB.ExecContext(ctx, `INSERT INTO made (user_id, deal_id, target, app, ref, title, url, state, error, checked_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		uid, m.DealID, m.Target, m.App, m.Ref, m.Title, m.URL, m.State, m.Error, m.CheckedAt)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	if m.CreatedAt == "" {
		m.CreatedAt = Now()
	}
	return nil
}

// SetMadeState records what a linked thing is now (sent, signed, paid…).
func (s *Store) SetMadeState(ctx context.Context, uid, id int64, state, title string) error {
	_, err := s.DB.ExecContext(ctx, `UPDATE made SET state = ?, title = CASE WHEN ? <> '' THEN ? ELSE title END, checked_at = ? WHERE user_id = ? AND id = ?`,
		state, title, title, Now(), uid, id)
	return err
}

// DeleteMade forgets a linked thing (it stays in the other app).
func (s *Store) DeleteMade(ctx context.Context, uid, id int64) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND id = ?`, uid, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DropMadeErrors forgets the failed tries of one kind once one worked.
func (s *Store) DropMadeErrors(ctx context.Context, uid, dealID int64, target string) {
	_, _ = s.DB.ExecContext(ctx, `DELETE FROM made WHERE user_id = ? AND deal_id = ? AND target = ? AND error <> ''`, uid, dealID, target)
}

// MadeToCheck returns linked things whose state may still change
// (documents out for signing, unpaid invoices), oldest look first.
func (s *Store) MadeToCheck(ctx context.Context, uid int64, limit int) ([]*Made, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+madeCols+` FROM made WHERE user_id = ? AND error = '' AND ref <> ''
		AND target IN ('signing', 'invoice', 'task') AND state NOT IN ('completed', 'signed', 'paid', 'void', 'voided', 'declined', 'done', 'cancelled')
		ORDER BY checked_at, id LIMIT ?`, uid, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Made{}
	for rows.Next() {
		var m Made
		if err := rows.Scan(&m.ID, &m.DealID, &m.Target, &m.App, &m.Ref, &m.Title, &m.URL, &m.State, &m.Error, &m.CheckedAt, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &m)
	}
	return out, rows.Err()
}

// AllDeals returns every deal a person has (for reports, people and exports).
func (s *Store) AllDeals(ctx context.Context, uid int64) ([]*Deal, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT `+dealCols+dealFrom+` WHERE d.user_id = ? ORDER BY d.created_at, d.id`, uid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Deal{}
	for rows.Next() {
		d, err := scanDeal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
