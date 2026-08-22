// Package seed creates the demo accounts a live walkthrough needs.
//
// Registration always produces role `user`, by design — so without seeding there
// is no way to exercise the collector verification step or any admin endpoint.
package seed

import (
	"context"
	"errors"
	"fmt"

	"zoa/backend/internal/auth"
	"zoa/backend/internal/models"
	"zoa/backend/internal/store"
)

// DemoPassword is shared by every seeded account. Safe only because seeding is
// opt-in via -seed-demo and these accounts exist purely for the walkthrough.
const DemoPassword = "zoa1234"

// DemoUser describes one account to create.
type DemoUser struct {
	Phone string
	Name  string
	Role  string
}

// DemoUsers is the cast for the demo: a recycler to submit, two collectors to
// verify, partner staff to accept a code at checkout, and an admin for stats.
//
// Phones stay in E.164 with the +254 prefix because login matches the stored
// string exactly — the same number saved as 0712000005 would not authenticate.
//
// The recycler and partner-staff entries are not padding. POST /submissions and
// POST /submissions/classify both sit behind DenyRole(collector), so a collector
// can neither submit nor photograph; drop the recycler and there is nothing for
// a collector to verify. Removing partner staff removes the redemption step.
var DemoUsers = []DemoUser{
	{Phone: "+254712000001", Name: "Amina Wanjiru", Role: models.RoleUser},
	{Phone: "+254712000002", Name: "Joseph Kariuki", Role: models.RoleCollector},
	{Phone: "+254712000003", Name: "Naivas Till 4", Role: models.RolePartnerStaff},
	{Phone: "+254712000004", Name: "Zoa Operations", Role: models.RoleAdmin},
	{Phone: "+254712000005", Name: "Grace Achieng", Role: models.RoleCollector},
}

// Result reports what seeding did.
type Result struct {
	Created []DemoUser
	Skipped []DemoUser
}

// Users creates any missing demo accounts. Existing accounts are left untouched,
// so seeding is safe to re-run and will not reset a password mid-demo.
func Users(ctx context.Context, users *store.UserStore) (Result, error) {
	hash, err := auth.HashPassword(DemoPassword)
	if err != nil {
		return Result{}, fmt.Errorf("hash demo password: %w", err)
	}

	var result Result
	for _, demo := range DemoUsers {
		_, err := users.ByPhone(ctx, demo.Phone)
		if err == nil {
			result.Skipped = append(result.Skipped, demo)
			continue
		}
		if !errors.Is(err, store.ErrNotFound) {
			return result, fmt.Errorf("look up %s: %w", demo.Phone, err)
		}

		if _, err := users.Create(ctx, demo.Phone, demo.Name, hash, demo.Role); err != nil {
			return result, fmt.Errorf("create %s: %w", demo.Phone, err)
		}
		result.Created = append(result.Created, demo)
	}
	return result, nil
}
