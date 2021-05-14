package dsl

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Leaf defines a leaf of logic
type Leaf struct {
	Field    string      `json:"field"`
	Function *string     `json:"function"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
}

// LogicEntry defines a node logic
type LogicEntry struct {
	Left   *LogicEntry `json:"left,omitempty"`
	Right  *LogicEntry `json:"right,omitempty"`
	Symbol string      `json:"symbol,omitempty"`

	Logic   *LogicEntry `json:"logic,omitempty"`
	Compare *Leaf       `json:"compare,omitempty"`
}

// EsQueryBuilder returns a elasticsearch query string from logic tree
func EsQueryBuilder(root *LogicEntry, keywordFields, textFields map[string]int) (strQuery string) {
	if root.Compare != nil {
		boolES := "'"
		if root.Compare.Function == nil {
			val, ok := root.Compare.Value.(string)
			if root.Compare.Value != nil && ok == true && val == "" {
				if root.Compare.Operator == "~" {
					root.Compare.Operator = "="
				} else if root.Compare.Operator == "!~" {
					root.Compare.Operator = "!="
				}
			}
			if root.Compare.Operator == "=" {
				if root.Compare.Value != nil {
					boolES = "must"
					field := root.Compare.Field
					if _, ok := keywordFields[field]; ok == true {
						field += ".keyword"
					}
					var iValue interface{}
					tmpStr := ""
					tmpStr2 := ""
					if value, ok := root.Compare.Value.(string); ok == true {
						iValue = value
						tmpStr = `{
							"bool": {
								"%v": [
									{
										"query_string": {
											"default_field": "%v",
											"query": "%v"
										}
									}
								]
							}
						}`
						tmpStr2 = `{
							"bool": {
								"%v": [
									{"term": {"%v": "%v"}}
								]
							}
						}`
					} else if value, ok := root.Compare.Value.(float64); ok == true {
						iValue = value
						tmpStr = `{
							"bool": {
								"%v": [
									{
										"query_string": {
											"default_field": "%v",
											"query": %v
										}
									}
								]
							}
						}`
						tmpStr2 = `{
							"bool": {
								"%v": [
									{"term": {"%v": %v}}
								]
							}
						}`
					}
					if iValue != nil {
						if _, ok := textFields[root.Compare.Field]; ok == true {
							strQuery = fmt.Sprintf(tmpStr, boolES, field, iValue)
						} else {
							strQuery = fmt.Sprintf(tmpStr2, boolES, field, iValue)
						}
					}
				} else {
					boolES = "must_not"
					field := root.Compare.Field
					if _, ok := keywordFields[field]; ok == true {
						field += ".keyword"
					}
					strQuery = fmt.Sprintf(`{
						"bool": {
							"%v": [
								{
									"exists": {"field": "%v"}
								}
							]
						}
					}`, boolES, field)
				}
			} else if root.Compare.Operator == "!=" {
				if root.Compare.Value != nil {
					boolES = "must_not"
					field := root.Compare.Field
					if _, ok := keywordFields[field]; ok == true {
						field += ".keyword"
					}
					if value, ok := root.Compare.Value.(string); ok == true {
						strQuery = fmt.Sprintf(`{
							"bool": {
								"%v": [
									{"term": {"%v": "%v"}}
								]
							}
						}`, boolES, field, value)
					} else if value, ok := root.Compare.Value.(float64); ok == true {
						strQuery = fmt.Sprintf(`{
							"bool": {
								"%v": [
									{"term": {"%v": %v}}
								]
							}
						}`, boolES, field, value)
					}
				} else {
					boolES = "must"
					field := root.Compare.Field
					if _, ok := keywordFields[field]; ok == true {
						field += ".keyword"
					}
					strQuery = fmt.Sprintf(`{
						"bool": {
							"%v": [
								{
									"exists": {"field": "%v"}
								}
							]
						}
					}`, boolES, field)
				}
			} else if root.Compare.Operator == ">" {
				field := root.Compare.Field
				if value, ok := root.Compare.Value.(string); ok == true {
					strQuery = fmt.Sprintf(`{
						"range": {
							"%v": {
								"gt": "%v"
							}
						}
					}`, field, value)
				} else if value, ok := root.Compare.Value.(float64); ok == true {
					strQuery = fmt.Sprintf(`{
						"range": {
							"%v": {
								"gt": %v
							}
						}
					}`, field, value)
				}
			} else if root.Compare.Operator == ">=" {
				field := root.Compare.Field
				if value, ok := root.Compare.Value.(string); ok == true {
					strQuery = fmt.Sprintf(`{
						"range": {
							"%v": {
								"gte": "%v"
							}
						}
					}`, field, value)
				} else if value, ok := root.Compare.Value.(float64); ok == true {
					strQuery = fmt.Sprintf(`{
						"range": {
							"%v": {
								"gte": %v
							}
						}
					}`, field, value)
				}
			} else if root.Compare.Operator == "<" {
				field := root.Compare.Field
				if value, ok := root.Compare.Value.(string); ok == true {
					strQuery = fmt.Sprintf(`{
						"range": {
							"%v": {
								"lt": "%v"
							}
						}
					}`, field, value)
				} else if value, ok := root.Compare.Value.(float64); ok == true {
					strQuery = fmt.Sprintf(`{
						"range": {
							"%v": {
								"lt": %v
							}
						}
					}`, field, value)
				}
			} else if root.Compare.Operator == "<=" {
				field := root.Compare.Field
				if value, ok := root.Compare.Value.(string); ok == true {
					strQuery = fmt.Sprintf(`{
						"range": {
							"%v": {
								"lte": "%v"
							}
						}
					}`, field, value)
				} else if value, ok := root.Compare.Value.(float64); ok == true {
					strQuery = fmt.Sprintf(`{
						"range": {
							"%v": {
								"lte": %v
							}
						}
					}`, field, value)
				}
			} else if root.Compare.Operator == "~" {
				field := root.Compare.Field
				value := root.Compare.Value.(string)
				if _, ok := keywordFields[field]; ok == true {
					value = strings.ToLower(value)
				}
				value = "*" + value + "*"
				strQuery = fmt.Sprintf(`{
					"bool": {
						"must": [{
							"wildcard": {
								"%v": "%v"
							}
						}]
					}
				}`, field, value)
			} else if root.Compare.Operator == "!~" {
				field := root.Compare.Field
				value := root.Compare.Value.(string)
				if _, ok := keywordFields[field]; ok == true {
					value = strings.ToLower(value)
				}
				value = "*" + value + "*"
				strQuery = fmt.Sprintf(`{
					"bool": {
						"must_not": [{
							"wildcard": {
								"%v": "%v"
							}
						}]
					}
				}`, field, value)
			}
		} else if *root.Compare.Function == "in" {
			if root.Compare.Operator == "=" {
				boolES = "should"
			} else {
				boolES = "must_not"
			}
			field := root.Compare.Field
			if _, ok := keywordFields[field]; ok == true {
				field += ".keyword"
			}
			listES := []string{}
			values := root.Compare.Value.([]interface{})
			if len(values) != 0 {
				for i := range values {
					strTmp := fmt.Sprintf(`{"term": {"%v": "%v"}}`, field, values[i])
					listES = append(listES, strTmp)
				}
			} else {
				strTmp := fmt.Sprintf(`{
					"bool": {
						"must_not": [{
							"exists": {"field": "%v"}
						}]
					}
				}`, field)
				listES = append(listES, strTmp)
			}
			strQuery = fmt.Sprintf(`{
				"bool": {
					"%v": [ %v ]
				}
			}`, boolES, strings.Join(listES, ", "))
		} else if *root.Compare.Function == "notin" {
			if root.Compare.Operator == "=" {
				boolES = "must_not"
			} else {
				boolES = "should"
			}
			field := root.Compare.Field
			if _, ok := keywordFields[field]; ok == true {
				field += ".keyword"
			}
			listES := []string{}
			values := root.Compare.Value.([]interface{})
			if len(values) != 0 {
				for i := range values {
					strTmp := fmt.Sprintf(`{"term": {"%v": "%v"}}`, field, values[i])
					listES = append(listES, strTmp)
				}
			} else {
				strTmp := fmt.Sprintf(`{
					"bool": {
						"must_not": [{
							"exists": {"field": "%v"}
						}]
					}
				}`, field)
				listES = append(listES, strTmp)
			}
			strQuery = fmt.Sprintf(`{
				"bool": {
					"%v": [ %v ]
				}
			}`, boolES, strings.Join(listES, ", "))
		} else {
			if root.Compare.Operator == "=" {
				boolES = "must"
			} else {
				boolES = "must_not"
			}
			field := root.Compare.Field
			if _, ok := keywordFields[field]; ok == true {
				field += ".keyword"
			}
			strQuery = fmt.Sprintf(`{
				"bool": {
					"%v": [{
						"match": {
							"%v": "%v"
						}
					}]
				}
			}`, boolES, field, root.Compare.Value)
		}
	} else {
		if root.Logic.Symbol == "AND" {
			left := EsQueryBuilder(root.Logic.Left, keywordFields, textFields)
			right := EsQueryBuilder(root.Logic.Right, keywordFields, textFields)
			strQuery = fmt.Sprintf(`{
				"bool": {
					"must": [
						%v,
						%v
					]
				}
			}`, left, right)
		} else if root.Logic.Symbol == "OR" {
			left := EsQueryBuilder(root.Logic.Left, keywordFields, textFields)
			right := EsQueryBuilder(root.Logic.Right, keywordFields, textFields)
			strQuery = fmt.Sprintf(`{
				"bool": {
					"should": [
						%v,
						%v
					]
				}
			}`, left, right)
		}
	}

	var l1 map[string]map[string]interface{}
	if err := json.Unmarshal([]byte(strQuery), &l1); err == nil {
		if v, ok := l1["bool"]; ok == true {
			if _, ok := v["should"]; ok == true {
				l1["bool"]["minimum_should_match"] = 1
			}
		}
	} else {
		fmt.Printf("%v\n", err)
	}

	if buff, err := json.Marshal(l1); err == nil {
		strQuery = string(buff)
	}

	return
}

// GetListFieldsInSearchQuery get all searching fields in query
func GetListFieldsInSearchQuery(query LogicEntry) (result []string) {
	if query.Compare != nil && len(query.Compare.Field) != 0 {
		for i := range result {
			if result[i] == query.Compare.Field {
				return
			}
		}
		result = append(result, query.Compare.Field)
	} else if query.Logic != nil && (query.Logic.Symbol == "AND" || query.Logic.Symbol == "OR") {
		result = append(result, GetListFieldsInSearchQuery(*query.Logic.Left)...)
		result = append(result, GetListFieldsInSearchQuery(*query.Logic.Right)...)
	}
	return
}
