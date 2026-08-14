package scan

// FilterDisplay drops rows a display-only preference says to hide. It must
// never be used on any kill target-resolution path — those always resolve
// against a full, unfiltered scan, so a hidden row never silently becomes
// unkillable; it's only ever missing from what's shown.
func FilterDisplay(services []Service, hideSystemProcesses, hidePrivilegedPorts bool) []Service {
	if !hideSystemProcesses && !hidePrivilegedPorts {
		return services
	}
	out := make([]Service, 0, len(services))
	for _, s := range services {
		if hideSystemProcesses && !s.Owned {
			continue
		}
		if hidePrivilegedPorts && s.Port < 1024 {
			continue
		}
		out = append(out, s)
	}
	return out
}
