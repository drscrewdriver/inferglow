// Copyright 2026 InferGlow Authors
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package approval

import "fmt"

// evaluatePolicy checks the request against its access policy. Returns
// a non-nil Decision if the policy resolves the request without needing
// a handler; returns nil if the handler should proceed.
func evaluatePolicy(req *Request) *Decision {
	p := req.Policy
	if p == nil {
		return nil
	}

	// Check denied list first.
	for _, denied := range p.DeniedCapabilities {
		if denied == req.Capability {
			return &Decision{
				Status:         DecisionDenied,
				Approved:       false,
				Reason:         fmt.Sprintf("capability %q is denied by access policy", req.Capability),
				PolicyOverride: true,
			}
		}
	}

	// Check allowed list.
	for _, allowed := range p.AllowedCapabilities {
		if allowed == req.Capability {
			return &Decision{
				Status:         DecisionApproved,
				Approved:       true,
				Reason:         fmt.Sprintf("capability %q is allowed by access policy", req.Capability),
				PolicyOverride: true,
			}
		}
	}

	// Check risk level.
	if p.MaxRiskLevel != "" && RiskExceeds(req.Risk, p.MaxRiskLevel) {
		return &Decision{
			Status:         DecisionDenied,
			Approved:       false,
			Reason:         fmt.Sprintf("risk %q exceeds max allowed %q", req.Risk, p.MaxRiskLevel),
			PolicyOverride: true,
		}
	}

	return nil
}

// MergePolicies combines multiple access policies into one. Denied lists
// are unioned, allowed lists are intersected, and the strictest risk level
// wins.
func MergePolicies(policies ...*AccessPolicy) *AccessPolicy {
	if len(policies) == 0 {
		return nil
	}
	result := &AccessPolicy{}
	deniedSet := make(map[string]bool)
	allowedSet := make(map[string]bool)
	first := true

	for _, p := range policies {
		if p == nil {
			continue
		}
		for _, d := range p.DeniedCapabilities {
			deniedSet[d] = true
		}
		if first {
			for _, a := range p.AllowedCapabilities {
				allowedSet[a] = true
			}
			first = false
		} else {
			// Intersect allowed.
			for k := range allowedSet {
				found := false
				for _, a := range p.AllowedCapabilities {
					if a == k {
						found = true
						break
					}
				}
				if !found {
					delete(allowedSet, k)
				}
			}
		}
		if p.MaxRiskLevel != "" && (result.MaxRiskLevel == "" || RiskExceeds(result.MaxRiskLevel, p.MaxRiskLevel)) {
			result.MaxRiskLevel = p.MaxRiskLevel
		}
		if p.RequireApproval {
			result.RequireApproval = true
		}
	}

	for d := range deniedSet {
		result.DeniedCapabilities = append(result.DeniedCapabilities, d)
	}
	for a := range allowedSet {
		result.AllowedCapabilities = append(result.AllowedCapabilities, a)
	}
	return result
}
