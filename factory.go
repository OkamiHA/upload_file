package profile

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"gitlab.viettelcyber.com/kian-core-v2/kian_library/extractor"
	"gitlab.viettelcyber.com/kian-core-v2/profiling-engine/aggregate"
	config2 "gitlab.viettelcyber.com/kian-core-v2/profiling-engine/config"
	"gitlab.viettelcyber.com/kian-core-v2/profiling-engine/node_processor"
	"gitlab.viettelcyber.com/kian-core-v2/profiling-engine/util"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	RawEvent           = "raw_event"
	fieldTypeOriginal  = "original"
	fieldTypeReference = "reference"

	functionCount = "count"
	functionSum   = "sum"
)

type (
	FieldGetter interface {
		// Get returns the value extracted from the json msg
		Get(msg []byte) (interface{}, bool)

		// GetTargetFieldName returns the field name
		GetTargetFieldName() string
	}
	originalFieldGetter struct {
		fieldName string
	}
	referenceFieldGetter struct {
		fieldName       string
		targetFieldName string
		extractorFunc   extractor.ExtractorFunc
	}
	// params holds common parameters/states used during the course of profiling
	params struct {
		id                                               string
		behaviorId                                       string
		config                                           *aggregate.ProfileConfig
		status                                           int64
		windowDuration, hopDuration                      time.Duration
		entityGetters, attributeGetters, categoryGetters []FieldGetter
		redisKey                                         string
		mongoDbName, mongoCollName                       string
		timestampField                                   string
		profileType                                      string
		function                                         string
	}
	// rawParams supports for parsing and building params quickly
	rawParams struct {
		ProfileTime   string `mapstructure:"profile_time"`
		SavingPeriod  string `mapstructure:"saving_period"`
		RedisKey      string `mapstructure:"redis_key"`
		MongoCollName string `mapstructure:"mongo_collection"`
	}
)

func newParams(config *aggregate.ProfileConfig, extractorSrv extractor.IService, behaviorConfig *aggregate.BehaviorConfig) (*params, error) {
	// start validating profile parameters
	var rawParams rawParams
	if err := mapstructure.Decode(config.Params, &rawParams); err != nil {
		return nil, errors.Wrap(err, "invalid profile params")
	}
	rawParams.ProfileTime = strings.TrimSpace(rawParams.ProfileTime)
	rawParams.SavingPeriod = strings.TrimSpace(rawParams.SavingPeriod)
	rawParams.RedisKey = strings.TrimSpace(rawParams.RedisKey)
	rawParams.MongoCollName = strings.TrimSpace(rawParams.MongoCollName)

	if rawParams.RedisKey == "" {
		return nil, errors.New("required redis_key param")
	}
	if rawParams.MongoCollName == "" {
		return nil, errors.New("required mongo_collection param")
	}

	// windowDuration
	rawProfTime := rawParams.ProfileTime
	if len(rawProfTime) == 0 {
		return nil, errors.New("required profile_time param")
	}
	windowDuration, err := util.ParseDurationExtended(rawProfTime)
	if err != nil {
		return nil, errors.Wrap(err, "parse profile_time param")
	}
	if windowDuration < 0 {
		return nil, errors.New("profile_time must be non-negative")
	}
	// saving period duration
	if len(rawParams.SavingPeriod) == 0 {
		return nil, errors.New("required saving_period param")
	}
	hopDuration, err := util.ParseDurationExtended(rawParams.SavingPeriod)
	if err != nil {
		return nil, errors.Wrap(err, "parse saving_period param")
	}
	if hopDuration <= 0 {
		return nil, errors.New("saving_period must be a positive number")
	}
	if windowDuration > 0 && hopDuration.Seconds() >= windowDuration.Seconds() {
		return nil, errors.New("saving_period must be less than profile_time")
	}
	if tmp := float64(windowDuration.Nanoseconds()) / float64(hopDuration.Nanoseconds()); math.Round(tmp) != tmp {
		return nil, errors.New("profile_time must be a multiple of saving_period")
	}

	// randomize saving period
	rangeRandomStr := config2.GlobalConfig.MongoRangeTimeRandom
	randomTime, err := util.GetRandomTime(rangeRandomStr, hopDuration)
	//randomTime = 0
	if err != nil {
		return nil, errors.Wrap(err, "get random time")
	}
	hopDuration = hopDuration + randomTime

	// entities, attributes and Categories
	var entityGetters, attributeGetters, categoryGetters []FieldGetter
	for _, entity := range config.Entities {
		getter, err := fieldGetterFactory(entity, extractorSrv)
		if err != nil {
			return nil, errors.Wrap(err, "in fieldGetterFactory")
		}
		entityGetters = append(entityGetters, getter)
	}
	for _, attribute := range config.Attributes {
		wrapper, err := fieldGetterFactory(attribute, extractorSrv)
		if err != nil {
			return nil, errors.Wrap(err, "in fieldGetterFactory")
		}
		attributeGetters = append(attributeGetters, wrapper)
	}
	for _, category := range config.Categories {
		wrapper, err := fieldGetterFactory(category, extractorSrv)
		if err != nil {
			return nil, errors.Wrap(err, "in fieldGetterFactory")
		}
		categoryGetters = append(categoryGetters, wrapper)
	}
	if config.ProfileType == profGlobalNew {
		if len(entityGetters) > 0 || len(attributeGetters) == 0 {
			return nil, errors.New("global_new: entity must be empty and attribute must not empty")
		}
	}
	// end evaluating profile parameters
	timestampField := config2.GlobalConfig.DefaultTimestampField
	if behaviorConfig.FieldConfig != nil {
		if rawTsField, ok := behaviorConfig.FieldConfig["timestamp_field"]; ok && len(rawTsField) != 0 {
			timestampField = rawTsField
		}
	}
	return &params{
		id:               config.Id,
		behaviorId:       behaviorConfig.Id,
		config:           config,
		status:           profStatusOff,
		windowDuration:   windowDuration,
		hopDuration:      hopDuration,
		entityGetters:    entityGetters,
		attributeGetters: attributeGetters,
		categoryGetters:  categoryGetters,
		redisKey:         rawParams.RedisKey,
		mongoDbName:      mongoDbName,
		mongoCollName:    rawParams.MongoCollName,
		timestampField:   timestampField,
		profileType:      config.ProfileType,
	}, nil
}

func (g *referenceFieldGetter) GetTargetFieldName() string {
	return g.targetFieldName
}

func (g *originalFieldGetter) GetTargetFieldName() string {
	return g.fieldName
}

func (g *originalFieldGetter) Get(msg []byte) (interface{}, bool) {
	// https://github.com/tidwall/gjson#validate-json
	// should place validation here or in Behavior or LogSource will be better?
	if !gjson.ValidBytes(msg) {
		return nil, false
	}
	val := gjson.GetBytes(msg, fmt.Sprintf("%s.%s", RawEvent, g.fieldName)).Value()
	if val == nil {
		return val, false
	}
	return val, true
}

func (g *referenceFieldGetter) Get(msg []byte) (interface{}, bool) {
	if !gjson.ValidBytes(msg) {
		return nil, false
	}
	val := gjson.GetBytes(msg, fmt.Sprintf("%s.%s", RawEvent, g.fieldName)).Value()
	if val == nil {
		return nil, false
	}
	extractedVal, err := g.extractorFunc.Extract(val)
	return extractedVal, err == nil
}

func fieldGetterFactory(obj *aggregate.Object, extSrv extractor.IService) (FieldGetter, error) {
	if len(obj.Name) == 0 {
		return nil, errors.New("required field_name")
	}
	switch obj.Type {
	case fieldTypeOriginal:
		return &originalFieldGetter{
			fieldName: obj.Name,
		}, nil
	case fieldTypeReference:
		var extInfo aggregate.ExtraExtractor
		if err := mapstructure.Decode(obj.ExtraData["extractor"], &extInfo); err != nil {
			return nil, errors.Wrap(err, "invalid extra_data.extractor")
		}
		if len(extInfo.OriginField) == 0 {
			return nil, errors.New("required origin_field")
		}
		extId := extInfo.ExtractorTypeName
		extFunc := extSrv.GetExtractorFor(extId)
		if extFunc == nil {
			return nil, errors.Errorf("not found extractor '%s'", extId)
		}
		return &referenceFieldGetter{
			fieldName:       extInfo.OriginField,
			targetFieldName: obj.Name,
			extractorFunc:   extFunc,
		}, nil
	}
	return nil, errors.Errorf("unsupported type '%s'", obj.Type)
}

func NewProfile(config *aggregate.ProfileConfig, behaviorConfig *aggregate.BehaviorConfig, in chan *node_processor.RawEvent, redisClient *redis.Client, mongoClient *mongo.Client, extractorSrv extractor.IService) (IProfile, error) {
	params, err := newParams(config, extractorSrv, behaviorConfig)
	if err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}
	batchRepo, err := NewMongoBatchRepo(mongoClient, params.mongoDbName, params.mongoCollName)
	if err != nil {
		return nil, errors.Wrap(err, "create a new batch repository")
	}
	// communicator := newRedisCommunicator(redisClient, params.redisKey)
	communicator := newKafkaCommunicator()
	switch config.ProfileType {
	case profTypeNew:
		return initNewProfile(params, in, batchRepo, communicator)
	case profTypeRare:
		return initRareProfile(params, in, batchRepo, communicator)
	case profTypeFirstSeen, profTypeLastSeen, profTypeAge, profTypeTimeline, profTypeGlobalAge, profTypeGlobalFirstSeen, profTypeGlobalLastSeen:
		return initTimelineProfile(params, in, batchRepo, communicator)
	case profTypeGlobal:
		return initGlobalProfile(params, in, batchRepo, communicator)
	case profTypeGlobalCountDistinct:
		return initGCDProfile(params, in, batchRepo, communicator)
	case profGlobalNew:
		return initGlobalNewProfile(params, in, batchRepo, communicator)
	case profTypePercentile:
		if len(params.attributeGetters) > 1 {
			return nil, errors.New("profile percentile require 1 attribute only")
		}
		return initPercentileProfile(params, in, batchRepo, communicator)
	default:
		return nil, errors.Errorf("unsupported profile type '%s'", config.ProfileType)
	}
}
