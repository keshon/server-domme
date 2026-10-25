package welcome

import (
	"fmt"
	"slices"

	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/perm"
)

// canRun keeps the welcome-specific exception inside this domain. Every
// configuration action, including future ones, requires an administrator.
func canRun(ctx *adapter.SlashInteractionContext, sub string) (bool, error) {
	if ctx.GuildID() == "" || ctx.UserID() == "" || ctx.UserID() == adapter.UnknownUserID {
		return false, nil
	}
	if config.IsDeveloper(ctx.Config, ctx.UserID()) {
		return true, nil
	}
	perms, err := ctx.MemberPermissions()
	if err != nil {
		return false, fmt.Errorf("welcome: get user permissions: %w", err)
	}
	if perms&perm.Administrator != 0 {
		return true, nil
	}
	if (sub != subMember && sub != subPreview) || ctx.Storage == nil {
		return false, nil
	}
	for _, role := range ctx.Storage.WelcomeOperators(ctx.GuildID()) {
		if slices.Contains(ctx.Invoker.Roles, role) {
			return true, nil
		}
	}
	return false, nil
}

func runAccess(ctx *adapter.SlashInteractionContext, opts options) error {
	roleID := roleIDOf(opts)
	_, hasAllow := opts[optAllow]
	if roleID == "" && !hasAllow {
		roles := ctx.Storage.WelcomeOperators(ctx.GuildID())
		if len(roles) == 0 {
			return respond(ctx, "Only administrators can welcome and preview. Allow a role with `/welcome access role:@Greeters allow:True`.")
		}
		return respond(ctx, "Can welcome and preview: "+mentionRoleIDs(roles)+" (plus administrators). Configuration stays administrator-only.")
	}
	if roleID == "" || !hasAllow {
		return respond(ctx, "Provide both `role` and `allow`: true to allow the role, false to remove access.")
	}
	if roleID == ctx.GuildID() {
		return respond(ctx, "Choose a specific role; @everyone cannot be allowed.")
	}
	allow := optBool(opts, optAllow)
	if err := ctx.Storage.SetWelcomeOperator(ctx.GuildID(), roleID, allow); err != nil {
		return fmt.Errorf("welcome: update operator role: %w", err)
	}
	if allow {
		return respond(ctx, fmt.Sprintf("<@&%s> can now welcome and preview. Configuration stays administrator-only.", roleID))
	}
	return respond(ctx, fmt.Sprintf("Removed <@&%s> welcome access. Other allowed roles and administrator permissions still apply.", roleID))
}
