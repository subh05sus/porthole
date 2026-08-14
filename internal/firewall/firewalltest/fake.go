// Package firewalltest provides a scriptable firewall.Manager test
// double, mirroring scan/scantest, kill/killtest, and proc/proctest.
package firewalltest

import (
	"context"

	"github.com/subh05sus/porthole/internal/firewall"
)

var _ firewall.Manager = (*FakeManager)(nil)

// FakeManager records every call and maintains an in-memory rule set, so
// tests can assert both "what was requested" and "what the rule set looks
// like afterward" without a real OS firewall.
type FakeManager struct {
	Rules []firewall.Rule

	ListErr      error
	ApplyErr     error
	RemoveErr    error
	RemoveAllErr error

	ApplyCalls     []firewall.Rule
	RemoveCalls    []firewall.Rule
	RemoveAllCalls int
}

func (f *FakeManager) List(ctx context.Context) ([]firewall.Rule, error) {
	if f.ListErr != nil {
		return nil, f.ListErr
	}
	out := make([]firewall.Rule, len(f.Rules))
	copy(out, f.Rules)
	return out, nil
}

func (f *FakeManager) Apply(ctx context.Context, rule firewall.Rule) error {
	f.ApplyCalls = append(f.ApplyCalls, rule)
	if f.ApplyErr != nil {
		return f.ApplyErr
	}
	for i, r := range f.Rules {
		if r.Name() == rule.Name() {
			f.Rules[i] = rule
			return nil
		}
	}
	f.Rules = append(f.Rules, rule)
	return nil
}

func (f *FakeManager) Remove(ctx context.Context, rule firewall.Rule) error {
	f.RemoveCalls = append(f.RemoveCalls, rule)
	if f.RemoveErr != nil {
		return f.RemoveErr
	}
	out := f.Rules[:0:0]
	for _, r := range f.Rules {
		if r.Name() != rule.Name() {
			out = append(out, r)
		}
	}
	f.Rules = out
	return nil
}

func (f *FakeManager) RemoveAll(ctx context.Context) error {
	f.RemoveAllCalls++
	if f.RemoveAllErr != nil {
		return f.RemoveAllErr
	}
	f.Rules = nil
	return nil
}
