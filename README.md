# Go Celery Api

This is a very simple Rest API that clients to add tasks to a celery queue via a POST request.

# Vagrant Setup

## Celery API
To run the API in a vagrant box , you must have [Vagrant](http://www.vagrantup.com/) and
[VirtualBox](https://www.virtualbox.org/) installed on your machine.  You will also need
Chef and Berkshelf, easily installed via [ChefDK](https://downloads.getchef.com/chef-dk/).

```sh
git clone https://github.com/divideandconquer/go-celery-api.git
cd go-celery-api

# Vagrant plugins to manage the network
vagrant plugin install vagrant-auto_network
vagrant plugin install vagrant-hostmanager
vagrant plugin install vagrant-triggers
vagrant plugin install vagrant-omnibus

vagrant up
```

## RabbitMQ Cluster
This repository requires the use of a rabbitmq / celery cluster.  To set one up follow the
directions for the [rabbitmq-cluster](https://github.com/turbine-web/rabbitmq-cluster) project.

Once you have the SSL keys generated make sure to copy the `testca/cacert.pem` and the client `key.pem`
and `cert.pem` files into this repo.

E.G.

```sh
cd go-celery-api
mkdir ssl
cp ../rabbitmq-cluster/ssl/testca/cacert.pem ssl/
cp ../rabbitmq-cluster/ssl/client/key.pem ssl/
cp ../rabbitmq-cluster/ssl/client/cert.pem ssl/
```

## Configuration
The default configuration works with the default configuration of the [rabbitmq-cluster](https://github.com/turbine-web/rabbitmq-cluster).
To override these defaults copy the `config/config.json.dist` file to `config/config.json`, edit the file appropriately
and then use the command line flag `-config=<path to config file>` to tell the go-celery-api to read that configuration.

The available configuration options are as follows:
* Cafile
  * The path to the Certificate Authority cert pem file
* Keyfile
  * The path to the client key.pem file
* Certfile
  * The path to the client cert.pem file
* Username
  * The rabbitmq username
* Password
  * The rabbitmq password
* Host
  * The rabbitmq dns name / ip address (likely this is the load balancer's host name)
* Port
  * The rabbitmq port (5672 or 5671 for ssl)
* CN
  * The common name on the SSL certs


## Running the API
Once you have a running rabbitmq / celery cluster, working ssl certs, and a vagrant box running go-celery-api
run the following commands to start the server:

```sh
# connect to server
vagrant ssh go

cd /vagrant

# run from binary
bin/go-celery-api

# run from src code
gom install
gom run src/go-celery-api.go

# you can also pass a configuration file to the go-celery-api:
gom run src/go-celery-api.go -config=./config/config.json
# or from the binary:
bin/go-celery-api  -config=./config/config.json

```
Note that in order for the default configuration to work you must add an entry for proxy to your `go` vagrant machine's
`/etc/hosts` file.

E.G.

```sh
# Note that the vagrant auto network plugin may have chosen a different ip for your proxy server.
sudo echo "10.20.1.6 proxy" >> /etc/hosts
```

## Posting to the API
To POST a task to the API make a POST to http://go:8080/tasks with a JSON body:

```
{
  "Name": "tasks.add", // the name of the celery task
  "Args": ["4", "8"]  // an array of arguments to pass to the celery task
  //"Kwargs": {...} //Key value store of kwargs to pass to the celery task
}
```

Using curl the command would look like this:

```sh
curl -H "Content-Type: application/json" --data '{"Name": "tasks.add", "Args": ["4", "8"]}' http://go:8080/tasks
```

The if the task was successfully added to the celery queue you will get the following response:

```
{
  "Status": "success"
}
```

# License
This module is licensed using the Apache-2.0 License:

```
Copyright (c) 2014, Kyle Boorky
```