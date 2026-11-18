package model

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// StorageTier is one step of a device's sensor data retention: readings are kept in
// buckets of Step seconds (0 = raw, one row per reading) for Keep seconds (0 = forever,
// only allowed on the last tier). When a tier's data gets older than Keep, it is merged
// into the next tier's buckets; after the last tier it is deleted.
type StorageTier struct {
	Step uint `json:"step"`
	Keep uint `json:"keep"`
}

// StorageSteps are the allowed bucket sizes in seconds. They all line up with UTC hours
// and days, so a bucket never straddles a coarser bucket of the same device.
var StorageSteps = []uint{0, 60, 300, 600, 900, 1800, 3600, 86400}

// DefaultStorageTiers keeps 5-minute buckets forever.
var DefaultStorageTiers = []StorageTier{{Step: 300, Keep: 0}}

const maxStorageTiers = 5

func (t StorageTier) StepDuration() time.Duration {
	return time.Duration(t.Step) * time.Second
}

func (t StorageTier) KeepDuration() time.Duration {
	return time.Duration(t.Keep) * time.Second
}

// ValidateStorageTiers checks that steps grow and divide each other, keep times grow,
// and only the last tier keeps data forever. An empty list means the default.
func ValidateStorageTiers(tiers []StorageTier) error {
	if len(tiers) == 0 {
		return nil
	}
	if len(tiers) > maxStorageTiers {
		return fmt.Errorf("at most %d storage tiers", maxStorageTiers)
	}
	for i, t := range tiers {
		if !slices.Contains(StorageSteps, t.Step) {
			return fmt.Errorf("tier %d: step %ds is not allowed", i+1, t.Step)
		}
		last := i == len(tiers)-1
		if t.Keep == 0 && !last {
			return fmt.Errorf("tier %d: only the last tier can keep data forever", i+1)
		}
		if t.Keep != 0 && t.Keep < t.Step {
			return fmt.Errorf("tier %d: keep time is shorter than its step", i+1)
		}
		if i == 0 {
			continue
		}
		prev := tiers[i-1]
		if t.Step <= prev.Step {
			return fmt.Errorf("tier %d: step must be larger than the previous one", i+1)
		}
		if prev.Step != 0 && t.Step%prev.Step != 0 {
			return fmt.Errorf("tier %d: step must be a multiple of the previous one", i+1)
		}
		if t.Keep != 0 && t.Keep <= prev.Keep {
			return errors.New("keep times must grow from tier to tier")
		}
	}
	return nil
}
