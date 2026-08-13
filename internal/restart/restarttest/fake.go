// Package restarttest provides a scriptable restart.Spawner test double,
// mirroring scan/scantest and kill/killtest — necessary because the real
// Spawner actually launches a new OS process.
package restarttest

import "github.com/subh05sus/porthole/internal/restart"

var _ restart.Spawner = (*FakeSpawner)(nil)

type FakeSpawner struct {
	Err   error
	Calls []restart.Plan
}

func (f *FakeSpawner) Spawn(plan restart.Plan) error {
	f.Calls = append(f.Calls, plan)
	return f.Err
}
