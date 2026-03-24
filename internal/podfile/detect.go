package podfile

import (
	"os"
	"path/filepath"
	"sort"
)

type detectionMatch struct {
	Name     string
	Marker   string
	Priority int
	Matched  int // how many detect.files were found
}

// DetectProjectType scans dir for marker files and returns the best matching
// template name and the file that triggered the match. Returns "minimal" if
// nothing specific is detected.
func DetectProjectType(dir string) (templateName string, markerFile string) {
	reg, err := LoadRegistry()
	if err != nil {
		return "minimal", ""
	}

	var matches []detectionMatch
	for _, t := range reg.Templates {
		if t.Detect == nil || len(t.Detect.Files) == 0 {
			continue
		}
		matched := 0
		firstMatch := ""
		for _, f := range t.Detect.Files {
			if _, err := os.Stat(filepath.Join(dir, f)); err == nil {
				matched++
				if firstMatch == "" {
					firstMatch = f
				}
			}
		}
		if t.Detect.All && matched != len(t.Detect.Files) {
			continue
		}
		if matched > 0 {
			matches = append(matches, detectionMatch{
				Name:     t.Name,
				Marker:   firstMatch,
				Priority: t.Detect.Priority,
				Matched:  matched,
			})
		}
	}

	if len(matches) == 0 {
		return "minimal", ""
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Priority != matches[j].Priority {
			return matches[i].Priority > matches[j].Priority
		}
		return matches[i].Matched > matches[j].Matched
	})

	return matches[0].Name, matches[0].Marker
}
