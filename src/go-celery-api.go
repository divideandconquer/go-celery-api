package main

import (
  "github.com/ant0ine/go-json-rest/rest"
  "github.com/bsphere/celery"
  "github.com/streadway/amqp"
  "log"
  "net/http"
  "crypto/tls"
  "crypto/x509"
  "io/ioutil"
  "time"
  "os"
  "encoding/json"
  "flag"
  "fmt"
)

// -- Main function --
// Sets up server and Tasks struct, starts listening on 8080
func main() {
  //set flags
  configFile := flag.String("config", "", "An optional filepath for a config file.")
  flag.Parse()

  if *configFile != "" {
  log.Printf("configFile: " + *configFile)
  }

  //setup tasks
  tasks := new(Tasks)
  tasks.ConfigFile = *configFile
  //load config
  tasks.SetupAmqpConnection()

  //setup resource handler and routes
  handler := rest.ResourceHandler{
  EnableRelaxedContentType: true,
  }
  err := handler.SetRoutes(
  rest.RouteObjectMethod("POST", "/tasks", tasks, "PostTask"),
  )
  if err != nil {
  log.Fatal(err)
  }
  log.Fatal(http.ListenAndServe(":8080", &handler))
}

// -- Struct Declarations --
type Task struct {
  Name   string
  Args   []string
  Kwargs map[string]interface{}
}
type TaskResult struct {
  Status string
}

type Tasks struct {
  Connection *amqp.Connection
  Config     *TaskConfig
  ConfigFile string
}

type TaskConfig struct {
  Uri       string
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
  CN       string
}

// -- Tasks Function Bindings --

// Retrieves the configuration from the config json
// builds the TaskConfig object and returns it
func (t *Tasks) GetConfig() *TaskConfig {

  if (t.Config == nil) {
    configuration := MainConfig{}

    if _, err := os.Stat(t.ConfigFile); os.IsNotExist(err) == false {
      log.Printf("Config file found - loading configuration")

      file, _ := os.Open(t.ConfigFile)
      decoder := json.NewDecoder(file)
      err := decoder.Decode(&configuration)
      if err != nil {
      log.Printf("Error decoding configuration: %s\n", err)
      }
    } else {
      //setup defaults
      log.Printf("No config file found, using defaults.")
      configuration.Cafile = "/vagrant/ssl/cacert.pem"
      configuration.Keyfile = "/vagrant/ssl/key.pem"
      configuration.Certfile = "/vagrant/ssl/cert.pem"
      configuration.Username = "admin"
      configuration.Password = "admin"
      configuration.Host = "proxy"
      configuration.Port = "5671"
      configuration.CN = "rabbit"
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
    cfg.ServerName = configuration.CN

    cert, _ := tls.X509KeyPair([]byte(clientCert), []byte(clientKey))
    cfg.Certificates = append(cfg.Certificates, cert)

    result := new(TaskConfig)
    result.TlsConfig = cfg
    result.Uri = fmt.Sprintf("amqps://%s:%s@%s:%s/", configuration.Username, configuration.Password, configuration.Host, configuration.Port)
    t.Config = result
  }
  return t.Config
}

//  This function will return a amqp channel as well
//  as setup the reconnect and retry logic in case
//  the server its connected to becomes unavailable.
func (t *Tasks) SetupAmqpConnection() *amqp.Connection {
  config := t.GetConfig()
  conn, err := amqp.DialTLS(config.Uri, config.TlsConfig)

  //if err retry until connected.
  if (err != nil) {
    log.Printf("Error Connecting to amqp: %s\n", err)
    time.Sleep(1 * time.Second)
    conn = t.SetupAmqpConnection()
  } else {
    log.Printf("Connected to Rabbit")
    //Setup Reconnect logic
    go func() {
      log.Printf("closing: %s \n", <-conn.NotifyClose(make(chan *amqp.Error)))
      time.Sleep(1 * time.Second)
      conn = t.SetupAmqpConnection()
    }()
  }

  t.Connection = conn
  // return the connection
  return conn
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
  } else {
    log.Printf("postTask - '%s'", task.Name)
    // we have a payload, create the celery task
    celeryTask, err := celery.NewTask(task.Name, task.Args, task.Kwargs)
    if err != nil {
      log.Printf("Could not create celery task: %+v \n", err)
      rest.Error(w, "Failed to create the celery task requested.", http.StatusBadRequest)
    } else {
        //we have created a task, now open a channel
      ch , err := t.Connection.Channel()
      if err != nil {
        log.Printf("Could not open channel: %+v \n", err)
        rest.Error(w, "Error connecting to celery", http.StatusInternalServerError)
      } else {
        //we have a channel so publish the task
        defer ch.Close()
        err = celeryTask.Publish(ch, "", "celery")
        if err != nil {
          log.Printf("Could not publish celery task: %+v \n", err)
          rest.Error(w, "Error publishing task to celery", http.StatusInternalServerError)
        } else {
          // Success!
          result := TaskResult{"success"}
          w.WriteJson(&result)
        }
      }
    }
  }
}
