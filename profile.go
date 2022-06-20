package profile

import (
	"fmt"
	"github.com/pkg/errors"
	"gitlab.viettelcyber.com/kian-core-v2/profiling-engine/aggregate"
	"gitlab.viettelcyber.com/kian-core-v2/profiling-engine/util"
	"math"
	"strconv"
	"time"
)

const (
	// profile type
	profTypeNew                 = "new"
	profTypeRare                = "rare"
	profTypeAge                 = "age"
	profTypeFirstSeen           = "firstseen"
	profTypeLastSeen            = "lastseen"
	profTypeGlobal              = "global"
	profTypeTimeline            = "timeline"
	profTypeGlobalAge           = "global_age"
	profTypeGlobalFirstSeen     = "global_firstseen"
	profTypeGlobalLastSeen      = "global_lastseen"
	profTypeGlobalCountDistinct = "global_count_distinct"
	profGlobalNew               = "global_new"
	profTypePercentile          = "percentile"
	profTypeAggregatedValue     = "agg"

	// profile status
	profStatusOff = 0
	profStatusOn  = 1

	// profile update params
	ProfParamUploadPeriod = "upload_period"
	ProfParamSavingPeriod = "saving_period"
	ProfParamProfileTime  = "profile_time"

	defDigestWeight      = 1
	defDigestCompression = 1000
)

type (
	IProfile interface {
		// Start starts running a profile that in turn runs its internal workers and set status to profStatusOn.
		Start() error
		// Stop stops a running profile that can be resumed later and set status to profStatusOff.
		Stop()
		// Close likes Stop but close all internal resources that means it cannot be restarted later
		// if cleanExternalResources set to true, related profile database (e.g. in Mongo) will be deleted.
		Close(cleanExternalResources bool)
		// GetConfig returns the configuration used for creation. It may not reflect the current profile states because of update.
		GetConfig() *aggregate.ProfileConfig
		GetBehaviorId() string
		GetStatus() int64
		UpdateProfileParams(param map[string]interface{}) error

		GetModelStats() *ModelStats
		RotateProfileBatches() error
	}

	commonProfile struct {
		*params
		builderWorker, communicatorWorker IWorker
		writerWorker                      IWriter
	}
)

func (p *commonProfile) Start() error {
	p.builderWorker.Start()
	p.writerWorker.Start()
	p.communicatorWorker.Start()
	p.status = profStatusOn
	return nil
}

func (p *commonProfile) Stop() {
	p.builderWorker.Stop()
	p.writerWorker.Stop()
	p.communicatorWorker.Stop()
	p.status = profStatusOff
}

func (p *commonProfile) Close(cleanExternalResources bool) {
	p.builderWorker.Close(cleanExternalResources)
	p.writerWorker.Close(cleanExternalResources)
	p.communicatorWorker.Close(cleanExternalResources)
	p.status = profStatusOff
}

func (p *commonProfile) GetStatus() int64 {
	return p.status
}

func (p *commonProfile) GetConfig() *aggregate.ProfileConfig {
	return p.config
}

func (p *commonProfile) UpdateProfileParams(params map[string]interface{}) error {
	const (
		updateWindowDurationFlag = 1 << iota
		updateHopDurationFlag
	)
	var (
		updateParamsMask uint8
		windowDuration   time.Duration
		hopDuration      time.Duration
		err              error
	)

	windowDuration = p.windowDuration
	if val, ok := params[ProfParamProfileTime]; ok {
		profileTime, ok := val.(string)
		if !ok {
			return errors.New("profile_time must be of type string")
		}
		windowDuration, err = util.ParseDurationExtended(profileTime)
		if err != nil {
			return errors.Wrap(err, "parse profile_time param")
		}
		switch p.profileType {
		case profTypeAge, profTypeFirstSeen, profTypeLastSeen, profTypeTimeline, profTypeGlobalAge, profTypeGlobalFirstSeen, profTypeGlobalLastSeen:
			if windowDuration < 0 {
				return errors.New("profile_time must be non-negative number")
			}
		default:
			if windowDuration <= 0 {
				return errors.New("profile_time must be positive number")
			}
		}
		updateParamsMask ^= updateWindowDurationFlag
	}

	if val, ok := params[ProfParamSavingPeriod]; ok {
		savingPeriod, ok := val.(string)
		if !ok {
			return errors.New("saving_period must be of type string")
		}
		hopDuration, err = util.ParseDurationExtended(savingPeriod)
		if err != nil {
			return errors.Wrap(err, "parse saving_period param")
		}
		if hopDuration <= 0 {
			return errors.New("saving_period must be positive number")
		}
		tmp := float64(windowDuration.Nanoseconds()) / float64(hopDuration.Nanoseconds())
		if math.Round(tmp) != tmp {
			return errors.New("profile_time must be a multiple of saving_period")
		}
		updateParamsMask ^= updateHopDurationFlag
	}

	// update hop duration
	if updateParamsMask&updateHopDurationFlag != 0 && hopDuration != p.hopDuration {
		p.builderWorker.Stop()
		p.hopDuration = hopDuration
		p.builderWorker.Start()
		p.config.Params[ProfParamSavingPeriod] = params[ProfParamSavingPeriod]
	}

	// update window duration
	if updateParamsMask&updateWindowDurationFlag != 0 && windowDuration != p.windowDuration {
		p.writerWorker.Stop()
		p.windowDuration = windowDuration
		p.writerWorker.Start()
		p.config.Params[ProfParamProfileTime] = params[ProfParamProfileTime]
	}
	return nil
}

func (p *commonProfile) GetBehaviorId() string {
	return p.behaviorId
}

func (p *commonProfile) GetModelStats() *ModelStats {
	switch p.profileType {
	case profTypePercentile:
		return nil
	}

	var sz uint64
	sz += p.builderWorker.GetAllocSize()
	sz += p.writerWorker.GetAllocSize()
	sz += p.communicatorWorker.GetAllocSize()

	pid, err := strconv.ParseInt(p.id, 10, 64)
	if err != nil {
		panic(fmt.Sprintf("invalid profile id %v", err)) // should panic
	}
	return &ModelStats{
		ID:        pid,
		Type:      p.profileType,
		AllocSize: sz,
	}
}

func (p *commonProfile) RotateProfileBatches() error {
	return p.writerWorker.RotateProfileBatches()
}
