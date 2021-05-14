# package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	cors "github.com/rs/cors/wrapper/gin"
	"github.com/sirupsen/logrus"
	"gitlab.visc.com/interactive-machine-learning-mvp/parser-query/pkg/dsl"
	"gitlab.visc.com/interactive-machine-learning-mvp/parser-query/pkg/option"
)

const (
	JSON = "application/json"
)

var logger = logrus.WithField("module", "api")

func Run() {
	opt := option.GetInstance()
	router := gin.Default()
	router.Use(cors.Default())
	parser := router.Group("/v1")
	{
		parser.POST("/parse", parse)
	}
	router.Run(fmt.Sprintf(":%v", opt.Port))
}
func parse(c *gin.Context) {
	opt := option.GetInstance()
	var m map[string]string
	c.BindJSON(&m)
	query_raw := m["query"]
	logger.Infof("Query input: %v", query_raw)
	logicTree := dsl.LogicEntry{}
	if err := json.Unmarshal([]byte(query_raw), &logicTree); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}
	keywordFields := opt.KeywordFields
	es_query := dsl.EsQueryBuilder(&logicTree, keywordFields, nil)
	logger.Debugf("Input query %v, %T", query_raw, query_raw)
	var results = map[string]string{
		"result": es_query,
	}
	c.JSON(200, results)
}
