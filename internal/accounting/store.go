// Package accounting implements durable operator budgets, not provider billing.
package accounting

import (
	"database/sql"
	"errors"
	"github.com/Apartment38/torii-gateway/internal/state"
	"math"
	"sync"
	"time"
)

type Budget struct {
	Requests      int64   `yaml:"requests,omitempty" json:"requests"`
	Tokens        int64   `yaml:"tokens,omitempty" json:"tokens"`
	EstimatedCost float64 `yaml:"estimated_cost,omitempty" json:"estimated_cost"`
}
type Budgets struct {
	Daily   Budget `yaml:"daily,omitempty" json:"daily"`
	Monthly Budget `yaml:"monthly,omitempty" json:"monthly"`
}
type Settings struct {
	Gateway Budgets            `yaml:"gateway,omitempty" json:"gateway"`
	Keys    map[string]Budgets `yaml:"keys,omitempty" json:"keys"`
}

func (b Budget) enabled() bool  { return b.Requests > 0 || b.Tokens > 0 || b.EstimatedCost > 0 }
func (b Budgets) enabled() bool { return b.Daily.enabled() || b.Monthly.enabled() }
func (s Settings) Validate() error {
	all := []Budgets{s.Gateway}
	for _, b := range s.Keys {
		all = append(all, b)
	}
	for _, b := range all {
		for _, v := range []Budget{b.Daily, b.Monthly} {
			if v.Requests < 0 || v.Tokens < 0 || v.Requests > 1e15 || v.Tokens > 1e15 || v.EstimatedCost < 0 || math.IsNaN(v.EstimatedCost) || math.IsInf(v.EstimatedCost, 0) {
				return errors.New("budgets require finite nonnegative limits (requests/tokens at most 10^15)")
			}
		}
	}
	return nil
}
func (s Settings) Limited(key string) bool { return s.Gateway.enabled() || s.Keys[key].enabled() }
func (s Settings) TokenLimited(key string) bool {
	return s.Gateway.Daily.Tokens > 0 || s.Gateway.Monthly.Tokens > 0 || s.Keys[key].Daily.Tokens > 0 || s.Keys[key].Monthly.Tokens > 0
}
func (s Settings) CostLimited(key string) bool {
	return s.Gateway.Daily.EstimatedCost > 0 || s.Gateway.Monthly.EstimatedCost > 0 || s.Keys[key].Daily.EstimatedCost > 0 || s.Keys[key].Monthly.EstimatedCost > 0
}

type LimitError struct {
	Reason string
	Reset  time.Time
}

func (e *LimitError) Error() string { return "operator quota unavailable: " + e.Reason }

var ErrStorage = errors.New("quota accounting storage unavailable")

type Amount struct {
	Requests  int64   `json:"requests"`
	Input     int64   `json:"input_tokens"`
	Output    int64   `json:"output_tokens"`
	Cost      float64 `json:"estimated_cost"`
	Unknown   int64   `json:"unknown_cost_attempts"`
	Estimated int64   `json:"estimated_tokens"`
}
type Reservation struct {
	ID, Key, Name, Provider, Model, Day, Month string
	Amount                                     Amount
}
type Store struct {
	db    *sql.DB
	mu    sync.Mutex
	clock func() time.Time
}

func Open(database *state.Database) (*Store, error) {
	if database == nil {
		return nil, ErrStorage
	}
	db, err := database.StateConnection()
	if err != nil {
		return nil, err
	}
	return &Store{db: db, clock: time.Now}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func periods(now time.Time) (string, string, time.Time, time.Time) {
	n := now.UTC()
	day := n.Format("2006-01-02")
	month := n.Format("2006-01")
	daily := time.Date(n.Year(), n.Month(), n.Day()+1, 0, 0, 0, 0, time.UTC)
	monthly := time.Date(n.Year(), n.Month()+1, 1, 0, 0, 0, 0, time.UTC)
	return day, month, daily, monthly
}
func readAmount(tx *sql.Tx, key, period string) (Amount, error) {
	var a Amount
	err := tx.QueryRow(`SELECT coalesce(sum(requests),0),coalesce(sum(input_tokens),0),coalesce(sum(output_tokens),0),coalesce(sum(cost),0),coalesce(sum(unknown_cost),0),coalesce(sum(estimated_tokens),0) FROM accounting_totals WHERE key_id=? AND period=?`, key, period).Scan(&a.Requests, &a.Input, &a.Output, &a.Cost, &a.Unknown, &a.Estimated)
	return a, err
}
func exceeded(b Budget, used, next Amount) bool {
	return b.Requests > 0 && next.Requests > b.Requests-used.Requests || b.Tokens > 0 && next.Input+next.Output > b.Tokens-used.Input-used.Output || b.EstimatedCost > 0 && (next.Unknown > 0 || used.Unknown > 0 || next.Cost > b.EstimatedCost-used.Cost)
}
func check(tx *sql.Tx, key, day, month string, settings Settings, next Amount, now time.Time) error {
	_, _, dr, mr := periods(now)
	for _, scope := range []struct {
		key string
		b   Budgets
	}{{key, settings.Keys[key]}, {"*", settings.Gateway}} {
		for _, p := range []struct {
			period string
			b      Budget
			reset  time.Time
		}{{day, scope.b.Daily, dr}, {month, scope.b.Monthly, mr}} {
			if !p.b.enabled() {
				continue
			}
			used, err := readAmount(tx, scope.key, p.period)
			if err != nil {
				return ErrStorage
			}
			if exceeded(p.b, used, next) {
				return &LimitError{Reason: "budget exhausted or pricing unknown", Reset: p.reset}
			}
		}
	}
	return nil
}
func (s *Store) CanReserve(key string, settings Settings, amount Amount) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return ErrStorage
	}
	defer tx.Rollback()
	now := s.clock()
	day, month, _, _ := periods(now)
	return check(tx, key, day, month, settings, amount, now)
}
func (s *Store) Reserve(key, name, provider, model string, settings Settings, amount Amount) (Reservation, error) {
	r := Reservation{ID: state.NewSecretID(), Key: key, Name: name, Provider: provider, Model: model, Amount: amount}
	if amount.Input < 0 || amount.Output < 0 || amount.Input > 1e12 || amount.Output > 1e12 || amount.Cost < 0 || math.IsNaN(amount.Cost) || math.IsInf(amount.Cost, 0) {
		return r, errors.New("invalid usage reservation")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Obtain the SQLite writer lock before reading totals. This prevents two
	// processes/pools from both passing a budget check on the same snapshot.
	tx, err := s.db.Begin()
	if err != nil {
		return r, ErrStorage
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE accounting_lock SET value=value WHERE id=1"); err != nil {
		return r, ErrStorage
	}
	now := s.clock()
	r.Day, r.Month, _, _ = periods(now)
	if err = check(tx, key, r.Day, r.Month, settings, amount, now); err != nil {
		return r, err
	}
	_, err = tx.Exec(`INSERT INTO accounting_reservations(id,key_id,key_name,provider,model,day,month,requests,input_tokens,output_tokens,cost,unknown_cost,estimated_tokens,status) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'reserved')`, r.ID, key, name, provider, model, r.Day, r.Month, amount.Requests, amount.Input, amount.Output, amount.Cost, amount.Unknown, amount.Estimated)
	if err != nil {
		return r, ErrStorage
	}
	if err = apply(tx, r, amount); err != nil {
		return r, ErrStorage
	}
	if err = tx.Commit(); err != nil {
		return r, ErrStorage
	}
	return r, nil
}
func apply(tx *sql.Tx, r Reservation, a Amount) error {
	for _, key := range []string{r.Key, "*"} {
		for _, period := range []string{r.Day, r.Month} {
			_, err := tx.Exec(`INSERT INTO accounting_totals(key_id,key_name,period,provider,model,requests,input_tokens,output_tokens,cost,unknown_cost,estimated_tokens) VALUES(?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(key_id,period,provider,model) DO UPDATE SET key_name=excluded.key_name,requests=requests+excluded.requests,input_tokens=input_tokens+excluded.input_tokens,output_tokens=output_tokens+excluded.output_tokens,cost=cost+excluded.cost,unknown_cost=unknown_cost+excluded.unknown_cost,estimated_tokens=estimated_tokens+excluded.estimated_tokens`, key, r.Name, period, r.Provider, r.Model, a.Requests, a.Input, a.Output, a.Cost, a.Unknown, a.Estimated)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
func (s *Store) Settle(r Reservation, actual *Amount) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return ErrStorage
	}
	defer tx.Rollback()
	status := "estimated"
	a := r.Amount
	if actual != nil {
		status = "reported"
		a = *actual
		a.Requests = r.Amount.Requests
		a.Estimated = 0
	}
	result, err := tx.Exec("UPDATE accounting_reservations SET status=? WHERE id=? AND status='reserved'", status, r.ID)
	if err != nil {
		return ErrStorage
	}
	count, err := result.RowsAffected()
	if err != nil {
		return ErrStorage
	}
	if count == 0 {
		return nil
	}
	if actual != nil {
		delta := Amount{Input: a.Input - r.Amount.Input, Output: a.Output - r.Amount.Output, Cost: a.Cost - r.Amount.Cost, Unknown: a.Unknown - r.Amount.Unknown, Estimated: -r.Amount.Estimated}
		if err = apply(tx, r, delta); err != nil {
			return ErrStorage
		}
	}
	// Completed reservations are not needed for budget checks: aggregates are
	// durable and authoritative. Retain unresolved reservations across restarts.
	if _, err = tx.Exec("DELETE FROM accounting_reservations WHERE id=?", r.ID); err != nil {
		return ErrStorage
	}
	if err = tx.Commit(); err != nil {
		return ErrStorage
	}
	return nil
}

type Summary struct {
	Name     string `json:"key_name"`
	Key      string `json:"key_id"`
	Period   string `json:"period"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Amount
}
type BudgetView struct {
	Key       string    `json:"key_id"`
	Period    string    `json:"period"`
	Reset     time.Time `json:"reset_at"`
	Limit     Budget    `json:"limit"`
	Used      Amount    `json:"used"`
	Remaining Budget    `json:"remaining"`
}

func (s *Store) Summary(settings Settings) ([]Summary, []BudgetView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock()
	day, month, dr, mr := periods(now)
	rows, err := s.db.Query("SELECT key_id,key_name,period,provider,model,requests,input_tokens,output_tokens,cost,unknown_cost,estimated_tokens FROM accounting_totals WHERE period IN (?,?) ORDER BY key_id,period,provider,model LIMIT 10000", day, month)
	if err != nil {
		return nil, nil, ErrStorage
	}
	summaries := []Summary{}
	for rows.Next() {
		var v Summary
		if err = rows.Scan(&v.Key, &v.Name, &v.Period, &v.Provider, &v.Model, &v.Requests, &v.Input, &v.Output, &v.Cost, &v.Unknown, &v.Estimated); err != nil {
			rows.Close()
			return nil, nil, err
		}
		summaries = append(summaries, v)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, nil, ErrStorage
	}
	defer tx.Rollback()
	views := []BudgetView{}
	all := map[string]Budgets{"*": settings.Gateway}
	for key, b := range settings.Keys {
		all[key] = b
	}
	for key, b := range all {
		for _, p := range []struct {
			id    string
			b     Budget
			reset time.Time
		}{{day, b.Daily, dr}, {month, b.Monthly, mr}} {
			used, err := readAmount(tx, key, p.id)
			if err != nil {
				return nil, nil, err
			}
			remaining := Budget{}
			if p.b.Requests > 0 {
				remaining.Requests = max(0, p.b.Requests-used.Requests)
			}
			if p.b.Tokens > 0 {
				remaining.Tokens = max(0, p.b.Tokens-used.Input-used.Output)
			}
			if p.b.EstimatedCost > 0 && used.Unknown == 0 {
				remaining.EstimatedCost = math.Max(0, p.b.EstimatedCost-used.Cost)
			}
			views = append(views, BudgetView{Key: key, Period: p.id, Reset: p.reset, Limit: p.b, Used: used, Remaining: remaining})
		}
	}
	return summaries, views, nil
}
func UnknownCapacity() error {
	_, _, reset, _ := periods(time.Now())
	return &LimitError{Reason: "supported model capacity or output limit required", Reset: reset}
}
