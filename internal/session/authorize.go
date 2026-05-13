package session

import (
	"context"

	"github.com/podspawn/podspawn/internal/policy"
)

// Authorize runs the Stage 8 deny-only policy gate. It is a strict
// pass-through to policy.Enforce — both the audit subject construction
// and the *PolicyError wrapping live in policy so the cmd entry points
// that gate non-service flows (rm, switch-branch, shell-attach) hit
// exactly the same shape as Create / End. Stage 8 has one production
// caller of policy.Enforce (this method); the free function is kept
// because future MCP / HTTP handlers will reach for the same shape
// without needing a *Service.
func (s *Service) Authorize(ctx context.Context, req policy.Request) error {
	return policy.Enforce(ctx, s.policy, s.audit, req)
}
