package interceptor

import (
	"net/url"

	"github.com/redpanda-data/console/backend/pkg/connector/model"
)

// ConsoleToKafkaConnectMongoDBHook sets connection.uri if not set
func ConsoleToKafkaConnectMongoDBHook(config map[string]any) map[string]any {
	_, exists := config["connection.uri"]
	if !exists {
		config["connection.uri"] = "mongodb://"
	}

	if config["connection.username"] != nil && config["connection.password"] != nil && config["connection.uri"] != nil {
		u, e := url.Parse(config["connection.uri"].(string))
		if e == nil {
			u.User = url.UserPassword(config["connection.username"].(string), config["connection.password"].(string))
		}
		config["connection.uri"] = u.String()
	}

	return config
}

// KafkaConnectToConsoleMongoDBHook adds username and password fields
func KafkaConnectToConsoleMongoDBHook(response model.ValidationResponse, _ map[string]any) model.ValidationResponse {
	response.Configs = append(response.Configs,
		model.ConfigDefinition{
			Definition: model.ConfigDefinitionKey{
				Name:         "connection.username",
				Type:         "STRING",
				DefaultValue: "",
				Importance:   "HIGH",
				Required:     false,
				DisplayName:  "MongoDB username",
			},
			Value: model.ConfigDefinitionValue{
				Name:              "connection.username",
				Value:             "",
				RecommendedValues: []string{},
				Visible:           true,
				Errors:            []string{},
			},
		},
		model.ConfigDefinition{
			Definition: model.ConfigDefinitionKey{
				Name:         "connection.password",
				Type:         "PASSWORD",
				DefaultValue: "",
				Importance:   "HIGH",
				Required:     false,
				DisplayName:  "MongoDB password",
			},
			Value: model.ConfigDefinitionValue{
				Name:              "connection.password",
				Value:             "",
				RecommendedValues: []string{},
				Visible:           true,
				Errors:            []string{},
			},
		},
	)

	return response
}
