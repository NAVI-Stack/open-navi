package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	coreskill "github.com/open-navi/navi/internal/navi/skill"
	"github.com/open-navi/navi/internal/store"
)

type SkillEntry = coreskill.SkillEntry
type Interface = coreskill.Interface
type SkillBuilder = coreskill.SkillBuilder
type BuildRequest = coreskill.BuildRequest
type GovernanceRequest = coreskill.GovernanceRequest
type GovernanceResponse = coreskill.GovernanceResponse

var RegisterInternalHandler = coreskill.RegisterInternalHandler

// RegisterSkillCreatorHandler exposes SkillBuilder through the internal skill transport.
func RegisterSkillCreatorHandler(id string, builder *SkillBuilder, db *sql.DB) {
	RegisterInternalHandler(id, "create", func(ctx context.Context, entry *SkillEntry, iface *Interface, args map[string]any) (any, error) {
		if builder == nil {
			return nil, fmt.Errorf("skill builder not configured")
		}

		capability, _ := args["capability"].(string)
		capability = strings.TrimSpace(capability)
		if capability == "" {
			return nil, fmt.Errorf("capability is required")
		}

		userContext, _ := args["context"].(string)
		userContext = strings.TrimSpace(userContext)
		if userContext == "" {
			userContext = "Create a new skill for: " + capability
		}
		chatID, _ := args["chat_id"].(string)

		gap := store.Gap{
			ID:     "manual-skill-gap-" + uuid.New().String(),
			Type:   store.GapTypeMissingSkill,
			Status: store.GapStatusClassified,
			Evidence: store.GapEvidence{
				ChatID:     chatID,
				ToolError:  "manual skill creation requested via skill-creator",
				RawContext: userContext,
			},
			ClassificationResult: store.GapClassification{
				GapClass:      store.GapTypeMissingSkill,
				ExpansionPath: "skill",
				Reason:        capability,
			},
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}
		if db != nil {
			if err := store.InsertGap(ctx, db, gap); err != nil {
				return nil, fmt.Errorf("insert synthetic gap: %w", err)
			}
		}

		result, err := builder.Build(ctx, BuildRequest{
			Gap:         gap,
			UserContext: userContext,
			ChatID:      chatID,
		})
		if err != nil {
			return nil, err
		}
		if result.GovernedPause {
			return map[string]any{
				"status":          "pending_governance",
				"reason":          result.GovernedReason,
				"proposal_id":     result.GovernedProposalID,
				"proposal_status": result.GovernedProposalStatus,
				"gap_closed":      result.GapClosed,
			}, nil
		}

		message := fmt.Sprintf("Created skill %s.", result.SkillID)
		if result.HubInstalled {
			message = fmt.Sprintf("Installed hub skill %s.", result.SkillID)
		}

		return map[string]any{
			"skill_id":   result.SkillID,
			"installed":  result.Installed,
			"message":    message,
			"skill_dir":  result.SkillDir,
			"gap_closed": result.GapClosed,
		}, nil
	})
}
