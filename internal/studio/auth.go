package studio

import (
	"context"
	"fmt"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// webAuth bridges gotd's interactive auth flow to HTTP: each step parks
// the flow on a channel until the matching /api/auth/* endpoint feeds it.
type webAuth struct {
	engine   *Engine
	phone    chan string
	code     chan string
	password chan string
}

func newWebAuth(e *Engine) *webAuth {
	return &webAuth{
		engine:   e,
		phone:    make(chan string, 1),
		code:     make(chan string, 1),
		password: make(chan string, 1),
	}
}

func (a *webAuth) wait(ctx context.Context, phase Phase, ch chan string) (string, error) {
	a.engine.setPhase(phase)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case v := <-ch:
		a.engine.setPhase(PhaseConnecting)
		return v, nil
	}
}

func (a *webAuth) Phone(ctx context.Context) (string, error) {
	return a.wait(ctx, PhaseNeedPhone, a.phone)
}

func (a *webAuth) Code(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
	return a.wait(ctx, PhaseNeedCode, a.code)
}

func (a *webAuth) Password(ctx context.Context) (string, error) {
	return a.wait(ctx, PhaseNeedPassword, a.password)
}

func (a *webAuth) AcceptTermsOfService(_ context.Context, _ tg.HelpTermsOfService) error {
	return nil
}

func (a *webAuth) SignUp(_ context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, fmt.Errorf("sign up is not supported; create the account in the Telegram app first")
}

// Submit feeds a value into whichever step the flow is waiting on.
func (e *Engine) SubmitAuth(step, value string) error {
	e.mu.Lock()
	wa := e.auth
	phase := e.phase
	e.mu.Unlock()
	if wa == nil {
		return fmt.Errorf("not connecting")
	}

	var (
		ch   chan string
		want Phase
	)
	switch step {
	case "phone":
		ch, want = wa.phone, PhaseNeedPhone
	case "code":
		ch, want = wa.code, PhaseNeedCode
	case "password":
		ch, want = wa.password, PhaseNeedPassword
	default:
		return fmt.Errorf("unknown auth step %q", step)
	}
	if phase != want {
		return fmt.Errorf("not waiting for %s (current phase: %s)", step, phase)
	}
	select {
	case ch <- value:
		return nil
	default:
		return fmt.Errorf("a value was already submitted")
	}
}
