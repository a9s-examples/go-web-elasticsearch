package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/cloudfoundry-community/go-cfenv"
	"github.com/mitchellh/mapstructure"
	elastic "gopkg.in/olivere/elastic.v5"
)

// Tweet is a structure used for serializing/deserializing data in Elasticsearch.
type Tweet struct {
	User     string                `json:"user"`
	Message  string                `json:"message"`
	Retweets int                   `json:"retweets"`
	Image    string                `json:"image,omitempty"`
	Created  time.Time             `json:"created,omitempty"`
	Tags     []string              `json:"tags,omitempty"`
	Location string                `json:"location,omitempty"`
	Suggest  *elastic.SuggestField `json:"suggest_field,omitempty"`
}

func handler(w http.ResponseWriter, r *http.Request) {
	var service cfenv.Service

	appEnv, errCfenv := cfenv.Current()
	if errCfenv == nil {
		for _, mappedServices := range appEnv.Services {
			for _, s := range mappedServices {
				service = s
				break
			}
		}
	}

	vcapServices := os.Getenv("VCAP_SERVICES")

	errDbMsg := ""
	msg := ""

	values := []int{}

	type Cred struct {
		Username string
		Password string
		Host     []string
	}
	var md mapstructure.Metadata
	var result Cred
	config := &mapstructure.DecoderConfig{
		Metadata: &md,
		Result:   &result,
	}
	decoder, err := mapstructure.NewDecoder(config)
	if err != nil {
		panic(err)
	}

	if err := decoder.Decode(service.Credentials); err != nil {
		panic(err)
	}

	u := "http://" + result.Host[0]

	msg += u
	msg += "\n"

	c := &http.Client{}
	req, err := http.NewRequest("GET", u, nil)
	req.SetBasicAuth(result.Username, result.Password)
	resp, err := c.Do(req)
	if err != nil {
		// handle error
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	msg += fmt.Sprintf("resp.Code = %s\n", resp.Status)
	msg += fmt.Sprintf("body = %s\n", body)

	client, err := elastic.NewClient(
		elastic.SetURL(u),
		elastic.SetMaxRetries(10),
		elastic.SetBasicAuth(result.Username, result.Password))
	if err == nil {
		// Use the IndexExists service to check if a specified index exists.
		exists, err := client.IndexExists("twitter").Do(context.TODO())
		if err != nil {
			//Handle error
			errDbMsg += err.Error()
		}
		if !exists {
			// Create a new index.
			createIndex, err := client.CreateIndex("twitter").Do(context.TODO())
			if err != nil {
				// Handle error
				errDbMsg += fmt.Sprintf("create index %v\n", err)
			}
			if !createIndex.Acknowledged {
				// Not acknowledged
			}
		}

		// Index a tweet (using JSON serialization)
		tweet1 := Tweet{User: "olivere", Message: "Take Five", Retweets: 0}
		put1, err := client.Index().
			Index("twitter").
			Type("tweet").
			Id("1").
			BodyJson(tweet1).
			Do(context.TODO())
		if err != nil {
			// Handle error
			errDbMsg += err.Error()
		}
		msg += fmt.Sprintf("Indexed tweet %s to index %s, type %s\n", put1.Id, put1.Index, put1.Type)

		// Index a second tweet (by string)
		tweet2 := `{"user" : "olivere", "message" : "It's a Raggy Waltz"}`
		put2, err := client.Index().
			Index("twitter").
			Type("tweet").
			Id("2").
			BodyString(tweet2).
			Do(context.TODO())
		if err != nil {
			// Handle error
			errDbMsg += err.Error()
		}
		msg += fmt.Sprintf("Indexed tweet %s to index %s, type %s\n", put2.Id, put2.Index, put2.Type)

		// Get tweet with specified ID
		get1, err := client.Get().
			Index("twitter").
			Type("tweet").
			Id("1").
			Do(context.TODO())
		if err != nil {
			// Handle error
			errDbMsg += fmt.Sprintf("get entry %v\n", err)
		}
		if get1.Found {
			msg += fmt.Sprintf("Got document %s in version %d from index %s, type %s\n", get1.Id, get1.Version, get1.Index, get1.Type)
		}

	}

	fmt.Fprintf(w, fmt.Sprintf("err = %v\nerrCfenv = %v\nerrDbMsg = %v\nenv VCAP_SERVICES: %s\nmsg = %v\ncredentials = %v\nvalues = %v", err, errCfenv, errDbMsg, vcapServices, msg, service.Credentials, values))
}

func main() {
	port := os.Getenv("PORT")
	if len(port) < 1 {
		port = "8080"
	}

	http.HandleFunc("/", handler)
	http.ListenAndServe(":"+port, nil)
}
