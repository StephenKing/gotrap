package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
)

// Global variable to store the configuration file
var configFile *string

// Init function to define arguments
func init() {
	configFile = flag.String("c", "./config.json", "Configuration file")
}

// The heart of gotrap.
func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Parse all arguments
	flag.Parse()

	fmt.Println("Hey, nice to meet you. Just wait a second. I will start up.")

	// Bootstrap configuration file
	conf := NewConfiguration(configFile)

	// Build the AMQP connection
	amqp := NewAmqpConnection(conf.Amqp.Host, conf.Amqp.Port, conf.Amqp.Username, conf.Amqp.Password, conf.Amqp.VHost)
	err := amqp.connect()

	// If we don`t get a AMQP connection we can exit here
	// Without AMQP connection gotrap is useless
	if err != nil {
		log.Fatalf("> AMQP connection not available: %v", err)
		os.Exit(1)
	}
	defer amqp.Connection.Close()

	// Declare AMQP exchange and queue and bind them togeter :)
	amqp.declareAndBind(conf.Amqp.Exchange, conf.Amqp.Queue, conf.Amqp.RoutingKey)

	// If this will fail we can exit here with the same reason like above
	// Without queue gotrap is useless
	if err != nil {
		log.Fatalf("> AMQP Declare and bind: %v", err)
		os.Exit(1)
	}

	// Get the consumer channel to get all messages
	messages, err := amqp.Channel.Consume(conf.Amqp.Queue, conf.Amqp.Identifier, false, false, false, false, nil)
	if err != nil {
		log.Fatalf("> AMQP Basic.consume: %v", err)
		os.Exit(1)
	}

	// Bootstrap a waitgroup
	// With this we are running as long as the go routines run
	var wg sync.WaitGroup
	wg.Add(1)

	// Limit number of concurrent patch requests here with a semaphore
	sem := make(chan bool, conf.Gotrap.Concurrent)

	// Start main go routine to receive messages by the AMQP broker
	go func() {
		defer wg.Done()

		// Get new messages
		for event := range messages {
			// Semaphore! Fill it
			sem <- true
			log.Printf("Add semaphore ... len: %d", len(sem))
			wg.Add(1)

			// One go routine per message
			go func() {
				defer func() {
					// Semaphore! Release it if this message was handled
					<-sem
					log.Printf("Release semaphore ... len: %d", len(sem))
					wg.Done()
				}()

				// Bootstrap the Github and Gerrit client ...
				githubClient := *NewGithubClient(&conf.Github)
				gerritClient := *NewGerritInstance(&conf.Gerrit)

				// ... and start handle the message!
				handleNewMessage(githubClient, gerritClient, *conf, event)
			}()
		}
	}()

	wg.Wait()

	fmt.Println("Our job is done. We have to go.")
}