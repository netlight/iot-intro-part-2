package main

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	influx "github.com/influxdata/influxdb/client/v2"
)

const (
	MyDB     = "wadus"
	username = "root"
	password = "root"
)

type Reading struct {
	DeviceId     string `json:"deviceId"`
	Measurements []struct {
		Type  string  `json:"type"`
		Value float64 `json:"value"`
	}
}

func defaultMqttHandler(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Unhandled message on topic %q - payload: %s\n", msg.Topic(), msg.Payload())
}
func (br *mqttInfluxBridge) storeSensorData(client mqtt.Client, msg mqtt.Message) {
	log.Printf("Got data on: %s\n", msg.Topic())
	var r Reading
	err := json.Unmarshal(msg.Payload(), &r)
	if err != nil {
		log.Println("Failed to unmarshal reading: ", err)
		return
	}
	bp, err := influx.NewBatchPoints(influx.BatchPointsConfig{
		Database: MyDB,
	})
	if err != nil {
		log.Println("Failed to create batch points: ", err)
		return
	}

	tags := map[string]string{
		"device": r.DeviceId,
	}
	for _, m := range r.Measurements {
		fields := map[string]interface{}{
			"value": m.Value,
		}

		pt, err := influx.NewPoint(m.Type, tags, fields, time.Now())
		if err != nil {
			log.Println("Failed to create point: ", err)
			return
		}
		bp.AddPoint(pt)
	}
	br.client.Write(bp)
}

type mqttInfluxBridge struct {
	client influx.Client
}

func main() {
	mqtt.ERROR = log.New(os.Stdout, "", 0)
	opts := mqtt.NewClientOptions().AddBroker("tcp://10.0.1.7:1883").SetClientID("influxdb-bridge")
	opts.SetKeepAlive(2 * time.Second)
	opts.SetDefaultPublishHandler(defaultMqttHandler)
	opts.SetPingTimeout(1 * time.Second)

	c := mqtt.NewClient(opts)
	if token := c.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	influxClient, err := influx.NewHTTPClient(influx.HTTPConfig{
		Addr:     "http://localhost:8086",
		Username: username,
		Password: password,
	})

	if err != nil {
		panic(err)
	}
	defer func() {
		err := influxClient.Close()
		if err != nil {
			log.Println("Error closing influx client: ", err)
		}
	}()
	bridge := mqttInfluxBridge{
		client: influxClient,
	}

	log.Println("Subscribing to sensordata/#")
	c.Subscribe("sensordata/#", 0, bridge.storeSensorData)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
}
