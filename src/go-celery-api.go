package main

import (
	"github.com/ant0ine/go-json-rest/rest"
	"github.com/bsphere/celery"
	"github.com/streadway/amqp"
	"log"
	"net/http"
	"fmt"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"time"
	"os"
	"encoding/json"
	"flag"
)

func main() {
	//set flags
	configFile := flag.String("config", "", "An optional filepath for a config file.")
	flag.Parse()

	if *configFile != "" {
		log.Printf("configFile: " + *configFile)
	}
	//load config
	config := getConfig(*configFile)

	//setup tasks
	ch := getAmqpChannel(config)
	tasks := Tasks{
		Channel: ch,
	}

	//setup resource handler and routes
	handler := rest.ResourceHandler{
		EnableRelaxedContentType: true,
	}
	err := handler.SetRoutes(
	rest.RouteObjectMethod("POST", "/tasks", &tasks, "PostTask"),
	)
	if err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":8080", &handler))
}

type Task struct {
	Name string
	Args []string
	Kwargs map[string]interface{}
}
type TaskResult struct {
	Status string
}

type Tasks struct {
	Channel *amqp.Channel
	Config TaskConfig
}

type TaskConfig struct {
	Uri string
	TlsConfig *tls.Config
}

type MainConfig struct {
	Cafile   string
	Keyfile  string
	Certfile string
	Username string
	Password string
	Host     string
	Port     string
}

// Retrieves the configuration from the config json
// builds the TaskConfig object and returns it
func getConfig(configFile string) *TaskConfig {
	//setup defaults
	configuration := MainConfig{}

	if _, err := os.Stat(configFile); os.IsNotExist(err) == false {
		log.Printf("Config file found - loading configuration")

		file, _ := os.Open(configFile)
		decoder := json.NewDecoder(file)
		err := decoder.Decode(&configuration)
		if err != nil {
			fmt.Println("Error decoding configuration:", err)
		}
	} else {
		log.Printf("No config file found, using defaults.")
		configuration.Cafile = "/vagrant/ssl/cacert.pem"
		configuration.Keyfile = "/vagrant/ssl/key.pem"
		configuration.Certfile = "/vagrant/ssl/cert.pem"
		configuration.Username = "admin"
		configuration.Password = "admin"
		configuration.Host = "proxy"
		configuration.Port = "5671"
	}

	rootCa, err := ioutil.ReadFile(configuration.Cafile)
	if err != nil { panic(err) }
	clientKey, err := ioutil.ReadFile(configuration.Keyfile)
	if err != nil { panic(err) }
	clientCert, err := ioutil.ReadFile(configuration.Certfile)
	if err != nil { panic(err) }

	cfg := new(tls.Config)
	cfg.RootCAs = x509.NewCertPool()
	cfg.RootCAs.AppendCertsFromPEM([]byte(rootCa))
	cfg.ServerName = "rabbit"

	cert, _ := tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
	cfg.Certificates = append(cfg.Certificates, cert)

	result := new(TaskConfig)
	result.TlsConfig = cfg
	result.Uri = fmt.Sprintf("amqps://%s:%s@%s:%s/", configuration.Username, configuration.Password, configuration.Host, configuration.Port)

	return result
}

//  This function will return a amqp channel as well
//  as setup the reconnect and retry logic in case
//  the server its connected to becomes unavailable.
func getAmqpChannel(config *TaskConfig) *amqp.Channel {

	conn, err := amqp.DialTLS(config.Uri, config.TlsConfig)
	var ch *amqp.Channel

	//if err retry until connected.
	if (err != nil) {
		fmt.Printf("Error Connecting to amqp: %s\n", err)
		time.Sleep(1 * time.Second)
		ch = getAmqpChannel(config)
	} else {
		log.Printf("Connected to Rabbit")
		ch, err = conn.Channel()
		if (err != nil) {
			panic(err)
		}
		//Setup Reconnect logic
		go func() {
			fmt.Printf("closing: %s \n", <-conn.NotifyClose(make(chan *amqp.Error)))
			time.Sleep(1 * time.Second)
			ch = getAmqpChannel(config)
		}()
	}

	// return the channel
	return ch
}

// POST http://go:8080/tasks
// {"Name":"<taskName", "Args": [<task params>]}
// E.G. {"Name": "tasks.add", "Args": ["4", "8"]}
func (t *Tasks) PostTask(w rest.ResponseWriter, r *rest.Request) {
	log.Printf("postTask - Received a task.")
	task := Task{}
	err := r.DecodeJsonPayload(&task)
	if err != nil {
		log.Printf("postTask - error reading payload.")
		rest.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("postTask - '" + task.Name + "'")

	status := "success"

	celeryTask, err := celery.NewTask(task.Name, task.Args, task.Kwargs)
	if err != nil {
		fmt.Printf("Could not create celery task: %+v \n", err)
		status = "failure"
	}

	err = celeryTask.Publish(t.Channel, "", "celery")
	if err != nil {
		fmt.Printf("Could not publish celery task: %+v \n", err)
		status = "failure"
	}

	result := TaskResult{status}
	w.WriteJson(&result)
}
