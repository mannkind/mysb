// Code generated by Wire. DO NOT EDIT.

//go:generate wire
//+build !wireinject

package main

import (
	"github.com/mannkind/paho.mqtt.golang.ext/cfg"
	"github.com/mannkind/paho.mqtt.golang.ext/di"
)

// Injectors from wire.go:

func initialize() *mqttClient {
	mqttConfig := cfg.NewMQTTConfig()
	config := NewConfig(mqttConfig)
	mqttFuncWrapper := di.NewMQTTFuncWrapper()
	mainMqttClient := newMqttClient(config, mqttFuncWrapper)
	return mainMqttClient
}
