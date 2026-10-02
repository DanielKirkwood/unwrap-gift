package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/DanielKirkwood/unwrap-gift/internal/api"
	"github.com/DanielKirkwood/unwrap-gift/internal/assign"
	"github.com/DanielKirkwood/unwrap-gift/internal/clients/smsclient"
	"github.com/DanielKirkwood/unwrap-gift/internal/db"
	"github.com/DanielKirkwood/unwrap-gift/internal/db/sqlc"
)

// drawStatusDraft, drawStatusAssigned, and drawStatusNotified match the
// migration's CHECK constraint values ('draft'/'assigned'/'notified') —
// there is no Go-side enum for these anywhere else in the codebase yet.
const (
	drawStatusDraft    = "draft"
	drawStatusAssigned = "assigned"
	drawStatusNotified = "notified"
)

// penceInPound converts draws.budget_amount's minor-unit pence into whole
// pounds for SMS display.
const penceInPound = 100

// storeDraws adapts *db.Store (and internal/assign) to api.DrawStore. It is
// the only file in this plan that imports internal/assign — internal/app
// is the composition root allowed to import both api and assign, the same
// as it's the only package allowed to import both api and db.
type storeDraws struct {
	store *db.Store
	sms   *smsclient.Client
}

func (s storeDraws) CreateDraw(
	ctx context.Context,
	drawerID int64,
	exchangeDate time.Time,
	budgetAmount int64,
) (api.Draw, error) {
	draw, err := s.store.Queries.CreateDraw(ctx, sqlc.CreateDrawParams{
		DrawerID:     drawerID,
		ExchangeDate: normalizeExchangeDate(exchangeDate),
		BudgetAmount: budgetAmount,
	})
	if err != nil {
		return api.Draw{}, fmt.Errorf("app: create draw: %w", err)
	}

	return toAPIDraw(draw), nil
}

// normalizeExchangeDate converts t to UTC and truncates it to midnight,
// since an exchange_date only ever carries day-level meaning (see the PRD's
// user flow) — this guarantees every stored value shares the exact same
// zone offset and a zero time-of-day/fractional-second component, so the
// DB's TEXT-column ORDER BY exchange_date DESC (which backs the history-
// window exclusion logic in buildHistory) sorts identically to true
// chronological order, regardless of what offset the client originally
// sent. Without this, two exchange_date values submitted with different
// client offsets aren't guaranteed to compare correctly as raw TEXT.
func normalizeExchangeDate(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

func (s storeDraws) GetDraw(ctx context.Context, drawerID, id int64) (api.Draw, error) {
	draw, err := s.store.Queries.GetDraw(ctx, id)
	if err != nil {
		return api.Draw{}, mapDrawErr(err)
	}
	if draw.DrawerID != drawerID {
		return api.Draw{}, api.ErrDrawNotFound
	}

	return toAPIDraw(draw), nil
}

func (s storeDraws) ListDrawsByDrawer(ctx context.Context, drawerID int64) ([]api.Draw, error) {
	rows, err := s.store.Queries.ListDrawsByDrawer(ctx, drawerID)
	if err != nil {
		return nil, fmt.Errorf("app: list draws by drawer: %w", err)
	}

	out := make([]api.Draw, len(rows))
	for i, row := range rows {
		out[i] = toAPIDraw(row)
	}

	return out, nil
}

func (s storeDraws) ListAssignmentsByDraw(ctx context.Context, drawerID, drawID int64) ([]api.Assignment, error) {
	draw, err := s.store.Queries.GetDraw(ctx, drawID)
	if err != nil {
		return nil, mapDrawErr(err)
	}
	if draw.DrawerID != drawerID {
		return nil, api.ErrDrawNotFound
	}

	rows, err := s.store.Queries.ListAssignmentsByDraw(ctx, drawID)
	if err != nil {
		return nil, fmt.Errorf("app: list assignments by draw: %w", err)
	}

	out := make([]api.Assignment, len(rows))
	for i, row := range rows {
		out[i] = toAPIAssignment(row)
	}

	return out, nil
}

// RunDraw computes and persists a constraint-respecting assignment for
// drawID, auto-relaxing the drawer's history window by one on
// ErrNoValidAssignment and retrying, down to zero, before giving up.
// Exclusions are never relaxed, regardless of how many retries happen.
func (s storeDraws) RunDraw(ctx context.Context, drawerID, drawID int64) (api.Draw, []api.Assignment, error) {
	draw, err := s.store.Queries.GetDraw(ctx, drawID)
	if err != nil {
		return api.Draw{}, nil, mapDrawErr(err)
	}
	if draw.DrawerID != drawerID {
		return api.Draw{}, nil, api.ErrDrawNotFound
	}
	if draw.Status != drawStatusDraft {
		return api.Draw{}, nil, api.ErrDrawAlreadyRun
	}

	drawer, err := s.store.Queries.GetDrawer(ctx, drawerID)
	if err != nil {
		return api.Draw{}, nil, mapDrawerErr(err)
	}

	members, err := s.store.Queries.ListMembersByDrawer(ctx, drawerID)
	if err != nil {
		return api.Draw{}, nil, fmt.Errorf("app: list members for run-draw: %w", err)
	}

	memberIDs := make([]assign.MemberID, len(members))
	phoneToMemberID := make(map[string]assign.MemberID, len(members))
	memberByID := make(map[int64]sqlc.Member, len(members))
	for i, m := range members {
		memberIDs[i] = assign.MemberID(m.ID)
		phoneToMemberID[m.PhoneNumber] = assign.MemberID(m.ID)
		memberByID[m.ID] = m
	}

	exclusions, err := s.buildExclusions(ctx, members, phoneToMemberID)
	if err != nil {
		return api.Draw{}, nil, err
	}

	window := drawer.HistoryWindowDraws
	for {
		history, historyErr := s.buildHistory(ctx, drawerID, drawID, window)
		if historyErr != nil {
			return api.Draw{}, nil, historyErr
		}

		result, assignErr := assign.Assign(memberIDs, exclusions, history)
		switch {
		case assignErr == nil:
			persistedDraw, assignments, persistErr := s.persistAssignment(ctx, draw, result)
			if persistErr != nil {
				return api.Draw{}, nil, persistErr
			}
			return s.finalizeNotifications(ctx, drawer.Name, persistedDraw, assignments, memberByID)
		case errors.Is(assignErr, assign.ErrTooFewMembers):
			return api.Draw{}, nil, api.ErrTooFewMembers
		case errors.Is(assignErr, assign.ErrNoValidAssignment) && window > 0:
			window--
			continue
		default: // ErrNoValidAssignment at window == 0, or ErrDuplicateMember (shouldn't happen: members come from one query)
			return api.Draw{}, nil, api.ErrNoValidAssignment
		}
	}
}

// buildExclusions resolves the drawer's members' global exclusion pairs
// into assign.ExclusionPair, only when both sides resolve to a member of
// this drawer — people excluded who aren't in this drawer are correctly
// irrelevant to this specific draw.
func (s storeDraws) buildExclusions(
	ctx context.Context,
	members []sqlc.Member,
	phoneToMemberID map[string]assign.MemberID,
) ([]assign.ExclusionPair, error) {
	var exclusions []assign.ExclusionPair

	for _, m := range members {
		rows, err := s.store.Queries.ListRelationshipsForPhoneNumber(ctx, sqlc.ListRelationshipsForPhoneNumberParams{
			PhoneNumberA: m.PhoneNumber,
			PhoneNumberB: m.PhoneNumber,
		})
		if err != nil {
			return nil, fmt.Errorf("app: list relationships for run-draw: %w", err)
		}

		for _, rel := range rows {
			other := rel.PhoneNumberB
			if rel.PhoneNumberB == m.PhoneNumber {
				other = rel.PhoneNumberA
			}

			otherID, ok := phoneToMemberID[other]
			if !ok {
				continue
			}

			exclusions = append(exclusions, assign.ExclusionPair{A: assign.MemberID(m.ID), B: otherID})
		}
	}

	return exclusions, nil
}

// buildHistory looks up the drawer's most recent window completed draws
// (excluding drawID itself) and converts their assignments into
// assign.HistoryPair. A window of zero skips the lookup entirely, ignoring
// history altogether.
func (s storeDraws) buildHistory(ctx context.Context, drawerID, drawID, window int64) ([]assign.HistoryPair, error) {
	if window == 0 {
		return nil, nil
	}

	pastDraws, err := s.store.Queries.ListRecentCompletedDrawsByDrawer(ctx, sqlc.ListRecentCompletedDrawsByDrawerParams{
		DrawerID: drawerID,
		ID:       drawID,
		Limit:    window,
	})
	if err != nil {
		return nil, fmt.Errorf("app: list recent completed draws for run-draw: %w", err)
	}

	var history []assign.HistoryPair
	for _, past := range pastDraws {
		assignments, assignmentsErr := s.store.Queries.ListAssignmentsByDraw(ctx, past.ID)
		if assignmentsErr != nil {
			return nil, fmt.Errorf("app: list assignments for history: %w", assignmentsErr)
		}

		for _, a := range assignments {
			history = append(history, assign.HistoryPair{
				Gifter: assign.MemberID(a.GifterMemberID),
				Giftee: assign.MemberID(a.GifteeMemberID),
			})
		}
	}

	return history, nil
}

// persistAssignment writes result as Assignment rows and flips the draw's
// status to 'assigned', inside a single transaction. It re-checks the
// draw's status transactionally before writing anything: RunDraw's own
// draft-status check (above, in the caller) runs before this transaction
// opens, so two concurrent RunDraw calls on the same draft draw could both
// pass that check before either commits. SQLite's single-connection pool
// serializes the two transactions, so without this re-check the second
// transaction would blindly try to insert assignment rows that collide
// with the first's already-committed ones (every full assignment over the
// same member set reuses every member as a gifter), surfacing as an
// unmapped UNIQUE-constraint 500 instead of a clean ErrDrawAlreadyRun.
func (s storeDraws) persistAssignment(
	ctx context.Context,
	draw sqlc.Draw,
	result map[assign.MemberID]assign.MemberID,
) (api.Draw, []api.Assignment, error) {
	var updated sqlc.Draw
	assignments := make([]api.Assignment, 0, len(result))

	err := s.store.WithTx(ctx, func(q *sqlc.Queries) error {
		current, getErr := q.GetDraw(ctx, draw.ID)
		if getErr != nil {
			return mapDrawErr(getErr)
		}
		if current.Status != drawStatusDraft {
			return api.ErrDrawAlreadyRun
		}

		for gifter, giftee := range result {
			row, createErr := q.CreateAssignment(ctx, sqlc.CreateAssignmentParams{
				DrawID:         draw.ID,
				GifterMemberID: int64(gifter),
				GifteeMemberID: int64(giftee),
			})
			if createErr != nil {
				return fmt.Errorf("app: create assignment: %w", createErr)
			}
			assignments = append(assignments, toAPIAssignment(row))
		}

		var statusErr error
		updated, statusErr = q.UpdateDrawStatus(ctx, sqlc.UpdateDrawStatusParams{
			ID:     draw.ID,
			Status: drawStatusAssigned,
		})
		if statusErr != nil {
			return fmt.Errorf("app: update draw status: %w", statusErr)
		}

		return nil
	})
	if err != nil {
		return api.Draw{}, nil, err
	}

	return toAPIDraw(updated), assignments, nil
}

// notifyMembers sends one SMS per assignment (to the gifter, about their
// giftee) and joins every per-member send error instead of stopping at
// the first failure — a transient failure for one member in a 20-person
// drawer shouldn't block the other 19 from being notified. Runs
// sequentially, not concurrently: group sizes are tiny (~5-30) and
// seven.io's SDK has no documented concurrency guarantees to lean on.
func (s storeDraws) notifyMembers(
	ctx context.Context,
	drawerName string,
	draw api.Draw,
	assignments []api.Assignment,
	memberByID map[int64]sqlc.Member,
) error {
	var errs []error

	for _, assignment := range assignments {
		gifter, giftee := memberByID[assignment.GifterMemberID], memberByID[assignment.GifteeMemberID]

		wishlist, err := s.store.Queries.ListWishlistItemsByPhoneNumber(ctx, giftee.PhoneNumber)
		if err != nil {
			errs = append(errs, fmt.Errorf("app: list wishlist items for giftee %d: %w", giftee.ID, err))
			continue
		}

		body := composeNotificationMessage(drawerName, draw, giftee.FullName, wishlist)
		if sendErr := s.sms.Send(ctx, gifter.PhoneNumber, body); sendErr != nil {
			errs = append(errs, fmt.Errorf("app: notify gifter %d: %w", gifter.ID, sendErr))
		}
	}

	return errors.Join(errs...)
}

// finalizeNotifications sends every notification SMS for a
// just-persisted draw and, only if every send succeeded, flips its
// status to 'notified'. A partial or total send failure leaves the draw
// at 'assigned' — the assignments it already computed stay valid and
// visible via ListAssignmentsByDraw, but RunDraw cannot be called again
// for this draw (it's no longer 'draft') and there is no retry-notify
// endpoint in v1. This is a known, surfaced gap (see this plan's Risks),
// not a silently swallowed one.
func (s storeDraws) finalizeNotifications(
	ctx context.Context,
	drawerName string,
	draw api.Draw,
	assignments []api.Assignment,
	memberByID map[int64]sqlc.Member,
) (api.Draw, []api.Assignment, error) {
	if notifyErr := s.notifyMembers(ctx, drawerName, draw, assignments, memberByID); notifyErr != nil {
		return api.Draw{}, nil, fmt.Errorf("%w: %w", api.ErrNotificationFailed, notifyErr)
	}

	updated, err := s.store.Queries.UpdateDrawStatus(ctx, sqlc.UpdateDrawStatusParams{
		ID:     draw.ID,
		Status: drawStatusNotified,
	})
	if err != nil {
		// Deliberately NOT wrapped in api.ErrNotificationFailed: every SMS
		// already sent successfully by this point, so a 502 ("notification
		// SMS messages failed to send") would misdiagnose what actually
		// happened. This falls through to api.Adapter's generic 500 and is
		// fully logged there (api.Adapter.logUnmapped) — the message makes
		// that distinction explicit for whoever reads the log, since the
		// draw is left at 'assigned' (not 'notified') either way, with no
		// retry-notify endpoint in v1 to recover it (same accepted gap as
		// a true notify failure — see this plan's Risks).
		return api.Draw{}, nil, fmt.Errorf(
			"app: all notification sms sent successfully but update draw status to notified failed (draw remains 'assigned'): %w",
			err,
		)
	}

	return toAPIDraw(updated), assignments, nil
}

// composeNotificationMessage builds the single SMS each gifter receives.
// The drawer name is prefixed because the same phone number can be a
// member of more than one drawer (the PRD's Should-priority "Multi-drawer
// support" row) — without it, two drawers notifying on the same day
// would read ambiguously. The wishlist is inlined rather than linked:
// there is no public, unauthenticated wishlist page to link to —
// wishlist_items are only readable by their own owner via the
// Auth-protected /wishlist-items endpoints (Phase 6).
func composeNotificationMessage(
	drawerName string,
	draw api.Draw,
	gifteeName string,
	wishlist []sqlc.WishlistItem,
) string {
	var b strings.Builder

	fmt.Fprintf(
		&b,
		"%s Secret Santa: you have %s! Exchange date: %s. Budget: %s.",
		drawerName,
		gifteeName,
		draw.ExchangeDate.Format("2 January 2006"),
		formatPence(draw.BudgetAmount),
	)

	if len(wishlist) == 0 {
		b.WriteString(" They haven't added a wishlist yet.")
		return b.String()
	}

	b.WriteString(" Wishlist: ")
	for i, item := range wishlist {
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(formatWishlistItem(item))
	}
	b.WriteString(".")

	return b.String()
}

func formatWishlistItem(item sqlc.WishlistItem) string {
	s := item.ItemName
	if item.Size.Valid {
		s += " (size " + item.Size.String + ")"
	}
	if item.Url.Valid {
		s += " - " + item.Url.String
	}
	return s
}

// formatPence renders minor-unit pence as a GBP string ("£25.00"). pence
// is always non-negative in practice — nothing validates that on
// draws.budget_amount today, so a negative budget would render with a
// stray sign in the pence component; not worth guarding for v1 (the
// organiser is the only writer of this field, via the hidden router).
func formatPence(pence int64) string {
	return fmt.Sprintf("£%d.%02d", pence/penceInPound, pence%penceInPound)
}

func toAPIDraw(d sqlc.Draw) api.Draw {
	return api.Draw{
		ID:           d.ID,
		DrawerID:     d.DrawerID,
		ExchangeDate: d.ExchangeDate,
		BudgetAmount: d.BudgetAmount,
		Status:       d.Status,
		CreatedAt:    d.CreatedAt,
		UpdatedAt:    d.UpdatedAt,
	}
}

func toAPIAssignment(a sqlc.Assignment) api.Assignment {
	return api.Assignment{
		ID:             a.ID,
		DrawID:         a.DrawID,
		GifterMemberID: a.GifterMemberID,
		GifteeMemberID: a.GifteeMemberID,
		CreatedAt:      a.CreatedAt,
	}
}

// mapDrawErr translates [sql.ErrNoRows] into api.ErrDrawNotFound.
func mapDrawErr(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return api.ErrDrawNotFound
	}

	return fmt.Errorf("app: draw store: %w", err)
}
