package account

import (
	"sync"
	"time"
)

type State string

const (
	StateHealthy     State = "healthy"
	StateCoolingDown State = "cooling_down"
	StateSuspended   State = "suspended"
	StateDisabled    State = "disabled"
)

type Credential struct {
	ID       string `json:"id"`
	Label    string `json:"label,omitempty"`
	Token    string `json:"-"`
	DeviceID string `json:"device_id"`
	TeamID   string `json:"team_id,omitempty"`
	ProxyURL string `json:"proxy_url,omitempty"`
}

type Account struct {
	Credential    Credential `json:"credential"`
	State         State      `json:"state"`
	CooldownUntil time.Time  `json:"cooldown_until,omitempty"`
	LastError     string     `json:"last_error,omitempty"`
}

type Router struct {
	mu       sync.Mutex
	accounts []*Account
	byID     map[string]*Account
	sessions map[string]string
	cursor   int
	now      func() time.Time
}

func NewRouter(accounts []Credential) *Router {
	r := &Router{byID:make(map[string]*Account),sessions:make(map[string]string),now:time.Now}
	for _, c := range accounts { r.addLocked(c) }
	return r
}

func (r *Router) addLocked(c Credential) {
	if c.ID == "" { return }
	if existing := r.byID[c.ID]; existing != nil {
		existing.Credential = c
		return
	}
	a := &Account{Credential:c,State:StateHealthy}
	r.accounts = append(r.accounts,a)
	r.byID[c.ID] = a
}

func (r *Router) Add(c Credential) {
	r.mu.Lock(); defer r.mu.Unlock()
	r.addLocked(c)
}

// Select returns an eligible account and pins it to session. An explicit ID
// overrides prior affinity only when that account is currently eligible.
func (r *Router) Select(session, explicitID string) *Account {
	r.mu.Lock(); defer r.mu.Unlock()
	now := r.now()
	if explicitID != "" {
		if a := r.byID[explicitID]; r.eligibleLocked(a,now) {
			if session != "" { r.sessions[session] = a.Credential.ID }
			return cloneAccount(a)
		}
		return nil
	}
	if session != "" {
		if id := r.sessions[session]; id != "" {
			if a := r.byID[id]; r.eligibleLocked(a,now) { return cloneAccount(a) }
			delete(r.sessions,session)
		}
	}
	if len(r.accounts) == 0 { return nil }
	for i:=0;i<len(r.accounts);i++ {
		idx := (r.cursor+i)%len(r.accounts)
		a := r.accounts[idx]
		if !r.eligibleLocked(a,now) { continue }
		r.cursor = (idx+1)%len(r.accounts)
		if session != "" { r.sessions[session] = a.Credential.ID }
		return cloneAccount(a)
	}
	return nil
}

func (r *Router) eligibleLocked(a *Account, now time.Time) bool {
	if a == nil { return false }
	if a.State == StateCoolingDown && !a.CooldownUntil.After(now) {
		a.State = StateHealthy
		a.CooldownUntil = time.Time{}
		a.LastError = ""
	}
	return a.State == StateHealthy
}

func (r *Router) MarkRateLimited(id string, until time.Time, detail string) {
	r.mu.Lock(); defer r.mu.Unlock()
	if a := r.byID[id]; a != nil {
		a.State=StateCoolingDown; a.CooldownUntil=until; a.LastError=detail
	}
}

func (r *Router) MarkSuspended(id, detail string) {
	r.mu.Lock(); defer r.mu.Unlock()
	if a := r.byID[id]; a != nil { a.State=StateSuspended; a.CooldownUntil=time.Time{}; a.LastError=detail }
}

func (r *Router) SetDisabled(id string, disabled bool) {
	r.mu.Lock(); defer r.mu.Unlock()
	if a := r.byID[id]; a != nil {
		if disabled { a.State=StateDisabled } else { a.State=StateHealthy; a.LastError=""; a.CooldownUntil=time.Time{} }
	}
}

func (r *Router) Snapshot() []Account {
	r.mu.Lock(); defer r.mu.Unlock()
	out:=make([]Account,0,len(r.accounts))
	for _,a:=range r.accounts { out=append(out,*cloneAccount(a)) }
	return out
}

func cloneAccount(a *Account) *Account {
	if a==nil { return nil }
	copy := *a
	return &copy
}
