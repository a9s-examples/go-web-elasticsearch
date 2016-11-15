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

type Credentials struct {
	Username string
	Password string
	Host     string
}

func parseCredentials() (credentials Credentials, err error) {
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
		return
	}

	if err = decoder.Decode(rawServiceCredentials()); err != nil {
		return
	}

	u := "http://" + result.Host[0]

	credentials.Username = result.Username
	credentials.Password = result.Password
	credentials.Host = u

	return
}

func rawServiceCredentials() map[string]interface{} {
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

	return service.Credentials
}

func httpGetRequest(credentials Credentials) (string, error) {
	c := &http.Client{}
	req, err := http.NewRequest("GET", credentials.Host, nil)
	req.SetBasicAuth(credentials.Username, credentials.Password)

	resp, err := c.Do(req)
	if err != nil {
		return "", err
		// handle error
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	msg := fmt.Sprintf("resp.Code = %s\n", resp.Status)
	msg += fmt.Sprintf("body = %s\n", body)

	return msg, nil
}

func handler(w http.ResponseWriter, r *http.Request) {
	errDbMsg := ""
	msg := ""
	values := []int{}

	credentials, err := parseCredentials()
	if err != nil {
		fmt.Printf("parseCredentials error = %v\n", err)
		panic(err)
	}

	//httpMsg, err := httpGetRequest(credentials)
	//if err != nil {
	//	errDbMsg += fmt.Sprintf("HTTP GET request failed: %s\n", err.Error())
	//}
	//msg += httpMsg

	client, err := elastic.NewClient(
		elastic.SetURL(credentials.Host),
		elastic.SetMaxRetries(10),
		elastic.SetBasicAuth(credentials.Username, credentials.Password),
		elastic.SetSniff(false))
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

	fmt.Fprintf(w, fmt.Sprintf("err = %v\nerrDbMsg = %v\nenv VCAP_SERVICES: %s\nmsg = %v\nvalues = %v", err, errDbMsg, os.Getenv("VCAP_SERVICES"), msg, values))
}

func main() {
	port := os.Getenv("PORT")
	if len(port) < 1 {
		port = "8080"
	}

	http.HandleFunc("/", handler)
	http.ListenAndServe(":"+port, nil)
}
