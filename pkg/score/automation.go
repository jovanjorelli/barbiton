package score

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
)

type AutomationCurveType string

const (
	CurveLinear      AutomationCurveType = "linear"
	CurveBezier      AutomationCurveType = "bezier"
	CurveExponential AutomationCurveType = "exponential"
	CurveStep        AutomationCurveType = "step"
)

type AutomationPoint struct {
	Time    float64 `json:"time"`
	Value   float64 `json:"value"`
	Curve   string  `json:"curve,omitempty"`
	Tension float64 `json:"tension,omitempty"`
}

type AutomationTrack []AutomationPoint

type AutomationMap map[string]AutomationTrack

func (at AutomationTrack) Len() int           { return len(at) }
func (at AutomationTrack) Less(i, j int) bool { return at[i].Time < at[j].Time }
func (at AutomationTrack) Swap(i, j int)      { at[i], at[j] = at[j], at[i] }

func (at AutomationTrack) SortedCopy() AutomationTrack {
	if len(at) == 0 {
		return nil
	}
	cp := make(AutomationTrack, len(at))
	copy(cp, at)
	sort.Sort(cp)
	return cp
}

func (at AutomationTrack) Interpolate(t float64) float64 {
	n := len(at)
	if n == 0 {
		return 0
	}
	if n == 1 || t <= at[0].Time {
		return at[0].Value
	}
	if t >= at[n-1].Time {
		return at[n-1].Value
	}

	idx := sort.Search(n, func(i int) bool {
		return at[i].Time > t
	})
	if idx <= 0 {
		return at[0].Value
	}
	if idx >= n {
		return at[n-1].Value
	}

	p0 := at[idx-1]
	p1 := at[idx]
	dt := p1.Time - p0.Time
	if dt <= 0 {
		return p1.Value
	}

	u := (t - p0.Time) / dt
	if u < 0 {
		u = 0
	} else if u > 1 {
		u = 1
	}

	curve := strings.ToLower(strings.TrimSpace(p1.Curve))
	if curve == "" {
		curve = strings.ToLower(strings.TrimSpace(p0.Curve))
	}
	if curve == "" {
		curve = string(CurveLinear)
	}

	tension := p1.Tension
	if tension == 0 {
		tension = p0.Tension
	}

	return interpolateSegment(p0.Value, p1.Value, u, curve, tension)
}

func interpolateSegment(v0, v1, u float64, curve string, tension float64) float64 {
	switch curve {
	case "step", "hold":
		if u < 1.0 {
			return v0
		}
		return v1
	case "exponential", "exp":
		if v0 > 0 && v1 > 0 {
			return v0 * math.Pow(v1/v0, u)
		}
		k := 2.5
		w := (math.Exp(k*u) - 1.0) / (math.Exp(k) - 1.0)
		return v0 + w*(v1-v0)
	case "bezier":
		tau := tension
		if tau < -1.0 {
			tau = -1.0
		} else if tau > 1.0 {
			tau = 1.0
		}
		p1 := 0.5 * (1.0 - tau)
		p2 := 0.5 * (1.0 + tau)
		invU := 1.0 - u
		w := 3.0*invU*invU*u*p1 + 3.0*invU*u*u*p2 + u*u*u
		return v0 + w*(v1-v0)
	default:
		return v0 + u*(v1-v0)
	}
}

func (am AutomationMap) Clone() AutomationMap {
	if am == nil {
		return nil
	}
	cp := make(AutomationMap, len(am))
	for k, v := range am {
		pts := make(AutomationTrack, len(v))
		copy(pts, v)
		cp[k] = pts
	}
	return cp
}

func (am AutomationMap) ValueAt(param string, t float64, defaultValue float64) float64 {
	if am == nil {
		return defaultValue
	}
	track, ok := am[param]
	if !ok || len(track) == 0 {
		return defaultValue
	}
	return track.Interpolate(t)
}

func (am AutomationMap) Has(param string) bool {
	if am == nil {
		return false
	}
	track, ok := am[param]
	return ok && len(track) > 0
}

func ValidateAutomationMap(am AutomationMap) []error {
	if am == nil {
		return nil
	}
	var errs []error
	for param, track := range am {
		if strings.TrimSpace(param) == "" {
			errs = append(errs, errors.New("automation parameter name cannot be empty"))
			continue
		}
		for i, pt := range track {
			if pt.Time < 0 || math.IsNaN(pt.Time) || math.IsInf(pt.Time, 0) {
				errs = append(errs, fmt.Errorf("automation %q point %d: invalid time %g", param, i, pt.Time))
			}
			if math.IsNaN(pt.Value) || math.IsInf(pt.Value, 0) {
				errs = append(errs, fmt.Errorf("automation %q point %d: invalid value %g", param, i, pt.Value))
			}
			c := strings.ToLower(strings.TrimSpace(pt.Curve))
			if c != "" && c != "linear" && c != "bezier" && c != "exponential" && c != "exp" && c != "step" && c != "hold" {
				errs = append(errs, fmt.Errorf("automation %q point %d: unknown curve %q", param, i, pt.Curve))
			}
			if math.IsNaN(pt.Tension) || math.IsInf(pt.Tension, 0) {
				errs = append(errs, fmt.Errorf("automation %q point %d: invalid tension %g", param, i, pt.Tension))
			}
		}
	}
	return errs
}
