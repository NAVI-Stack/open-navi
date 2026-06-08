package navi

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/open-navi/navi/internal/navi/experience"
	"gopkg.in/yaml.v3"
)

// ExperienceMode distinguishes the supported experience-layer operating modes.
type ExperienceMode string

const (
	ExperienceModeStandard ExperienceMode = "navi"
	ExperienceModeWizard   ExperienceMode = "wizard" // onboarding-only mode
)

// ExperienceProfile contains the experience profile for a given operating mode.
type ExperienceProfile struct {
	ID          ExperienceMode
	DisplayName string
	Profile     experience.ProfileConfig
}

// ExperienceManager manages the active operating mode and its experience profile.
type ExperienceManager struct {
	active   ExperienceMode
	profiles map[ExperienceMode]ExperienceProfile
	dir      string
	engine   *experience.Engine
}

// NormalizeExperienceMode collapses unsupported IDs into the standard NAVI experience.
func NormalizeExperienceMode(id ExperienceMode) ExperienceMode {
	switch strings.ToLower(strings.TrimSpace(string(id))) {
	case "", string(ExperienceModeStandard):
		return ExperienceModeStandard
	case string(ExperienceModeWizard):
		return ExperienceModeWizard
	default:
		return ExperienceModeStandard
	}
}

// NewExperienceManager initializes an ExperienceManager with profiles from the given directory.
func NewExperienceManager(dir string, defaultMode ExperienceMode) *ExperienceManager {
	em := &ExperienceManager{
		active:   NormalizeExperienceMode(defaultMode),
		profiles: defaultExperienceProfiles(),
		dir:      dir,
		engine:   experience.NewEngine(nil),
	}

	if err := em.Load(); err != nil {
		log.Printf("navi: failed to load experience profiles from %s: %v", dir, err)
	}
	if _, ok := em.profiles[em.active]; !ok {
		em.active = ExperienceModeStandard
	}

	return em
}

// Load reads all .yaml files from the experience profile directory.
func (em *ExperienceManager) Load() error {
	if em.dir == "" {
		return nil
	}

	entries, err := os.ReadDir(em.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || (!strings.HasSuffix(entry.Name(), ".yaml") && !strings.HasSuffix(entry.Name(), ".yml")) {
			continue
		}

		path := filepath.Join(em.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("experience: error reading %s: %v", path, err)
			continue
		}

		var profile experience.ProfileConfig
		if err := yaml.Unmarshal(data, &profile); err != nil {
			log.Printf("experience: error parsing %s: %v", path, err)
			continue
		}

		if strings.TrimSpace(profile.ID) == "" {
			profile.ID = strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		}
		overrideID, ok := supportedExperienceOverrideMode(ExperienceMode(profile.ID))
		if !ok {
			continue
		}
		profile.ID = string(overrideID)

		override := experienceProfileFromConfig(profile)
		em.profiles[overrideID] = mergeExperienceProfile(em.profiles[overrideID], override)
	}

	return nil
}

// Get retrieves the configuration for a given experience mode.
func (em *ExperienceManager) Get(id ExperienceMode) (ExperienceProfile, bool) {
	cfg, ok := em.profiles[NormalizeExperienceMode(id)]
	return cfg, ok
}

// Build derives the effective experience-layer control for a turn.
func (em *ExperienceManager) Build(ctx context.Context, id ExperienceMode, req experience.BuildRequest) (experience.RenderedControl, error) {
	cfg, ok := em.Get(id)
	if !ok {
		cfg = em.Active()
	}
	if strings.TrimSpace(req.Mode) == "" {
		req.Mode = string(cfg.ID)
	}
	if em.engine == nil {
		em.engine = experience.NewEngine(nil)
	}
	return em.engine.Build(ctx, cfg.Profile, req)
}

// Set switches the active experience mode if it exists.
func (em *ExperienceManager) Set(id ExperienceMode) error {
	id = NormalizeExperienceMode(id)
	if _, ok := em.profiles[id]; !ok {
		return fmt.Errorf("navi: unknown experience mode %q", id)
	}
	em.active = id
	return nil
}

// SetGovernanceBoundsProvider injects a bounds provider, re-initializing the engine.
func (em *ExperienceManager) SetGovernanceBoundsProvider(p experience.GovernanceBoundsProvider) {
	em.engine = experience.NewEngine(p)
}

// Active returns the currently active ExperienceProfile.
func (em *ExperienceManager) Active() ExperienceProfile {
	if cfg, ok := em.profiles[em.active]; ok {
		return cfg
	}
	return em.profiles[ExperienceModeStandard]
}

func defaultExperienceProfiles() map[ExperienceMode]ExperienceProfile {
	return map[ExperienceMode]ExperienceProfile{
		ExperienceModeStandard: experienceProfileFromConfig(experience.DefaultStandardProfile()),
		ExperienceModeWizard:   experienceProfileFromConfig(experience.DefaultWizardProfile()),
	}
}

func experienceProfileFromConfig(profile experience.ProfileConfig) ExperienceProfile {
	id := NormalizeExperienceMode(ExperienceMode(profile.ID))
	if profile.ID == "" {
		profile.ID = string(id)
	} else {
		profile.ID = strings.ToLower(strings.TrimSpace(profile.ID))
	}
	if profile.DisplayName == "" {
		if id == ExperienceModeWizard {
			profile.DisplayName = "Setup Wizard"
		} else {
			profile.DisplayName = "NAVI"
		}
	}
	for i := range profile.PersonaModules {
		profile.PersonaModules[i].OriginScope = "system"
		profile.PersonaModules[i].Source = "stored"
	}
	return ExperienceProfile{
		ID:          id,
		DisplayName: profile.DisplayName,
		Profile:     profile,
	}
}

func supportedExperienceOverrideMode(id ExperienceMode) (ExperienceMode, bool) {
	switch strings.ToLower(strings.TrimSpace(string(id))) {
	case string(ExperienceModeStandard):
		return ExperienceModeStandard, true
	case string(ExperienceModeWizard):
		return ExperienceModeWizard, true
	default:
		return "", false
	}
}

func mergeExperienceProfile(base, override ExperienceProfile) ExperienceProfile {
	if override.ID != "" {
		base.ID = NormalizeExperienceMode(override.ID)
	}
	if override.DisplayName != "" {
		base.DisplayName = override.DisplayName
	}
	if strings.TrimSpace(override.Profile.DisplayName) != "" {
		base.Profile.DisplayName = override.Profile.DisplayName
	} else if base.DisplayName != "" {
		base.Profile.DisplayName = base.DisplayName
	}
	base.Profile.ID = string(base.ID)

	if len(override.Profile.CoreIdentity.Traits) > 0 {
		if base.Profile.CoreIdentity.Traits == nil {
			base.Profile.CoreIdentity.Traits = map[string]float64{}
		}
		for trait, value := range override.Profile.CoreIdentity.Traits {
			base.Profile.CoreIdentity.Traits[trait] = value
		}
	}
	if len(override.Profile.PersonaModules) > 0 {
		base.Profile.PersonaModules = override.Profile.PersonaModules
	}
	base.Profile.OutputPreferences = mergeProfileOutputPreferences(base.Profile.OutputPreferences, override.Profile.OutputPreferences)
	if override.Profile.RoleContext.ActiveRole != "" {
		base.Profile.RoleContext.ActiveRole = override.Profile.RoleContext.ActiveRole
	}
	if override.Profile.RoleContext.TaskArchetype != "" {
		base.Profile.RoleContext.TaskArchetype = override.Profile.RoleContext.TaskArchetype
	}
	if override.Profile.RoleContext.DelegationMode != "" {
		base.Profile.RoleContext.DelegationMode = override.Profile.RoleContext.DelegationMode
	}
	return base
}

func mergeProfileOutputPreferences(base, override experience.OutputPreferences) experience.OutputPreferences {
	if override.PreferredLength != "" {
		base.PreferredLength = override.PreferredLength
	}
	if override.PreferredFormat != "" {
		base.PreferredFormat = override.PreferredFormat
	}
	if override.SummaryFirst != nil {
		value := *override.SummaryFirst
		base.SummaryFirst = &value
	}
	return base
}
