package scan

import "github.com/subh05sus/porthole/internal/project"

// Enrich fills in Project for each service by resolving its CWD, reusing
// detector's session cache across calls. Portable — every OS scanner calls
// this as its final step rather than reimplementing project resolution.
func Enrich(services []Service, detector *project.Detector) []Service {
	for i := range services {
		services[i].Project = detector.Detect(services[i].CWD)
	}
	return services
}
