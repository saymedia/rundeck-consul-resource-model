package main

import (
	"encoding/xml"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"time"

	consul "github.com/hashicorp/consul/api"
)

type Project struct {
	XMLName xml.Name `xml:"project"`
	Nodes   []Node
}

type Node struct {
	XMLName        xml.Name `xml:"node"`
	Name           string   `xml:"name,attr"`
	Hostname       string   `xml:"hostname,attr"`
	Tags           string   `xml:"tags,attr,omitempty"`
	Username       string   `xml:"username,attr"`
	DatacenterName string   `xml:"datacenter,attr"`
}

func main() {

	serviceNames := make([]string, 0, len(os.Args)-1)
	oneOffTags := make(map[string]string)
	consulAddr := os.Getenv("CONSUL_ADDRESS")
	consulScheme := os.Getenv("CONSUL_SCHEME")
	consulToken := os.Getenv("CONSUL_TOKEN")

	noMoreOptions := false
	for _, arg := range os.Args[1:] {
		if !noMoreOptions {
			if strings.HasPrefix(arg, "--tag-one=") {
				tagOneVal := arg[10:]
				parts := strings.SplitN(tagOneVal, ":", 2)
				oneOffTags[parts[0]] = parts[1]
				continue
			} else if arg == "--" {
				noMoreOptions = true
				continue
			} else if strings.HasPrefix(arg, "-") {
				fmt.Fprintf(os.Stderr, "Unknown option %s\n", arg)
				os.Exit(1)
			}
		}

		serviceNames = append(serviceNames, arg)
	}

	if len(serviceNames) < 1 {
		fmt.Fprintf(
			os.Stderr, "Usage: %s [options] service-names...\n", os.Args[0],
		)
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		fmt.Fprintf(
			os.Stderr, "  --tag-one=service:tagname\n",
		)
		fmt.Fprintf(os.Stderr, "\n")
		os.Exit(1)
	}

	if consulAddr == "" {
		consulAddr = "127.0.0.1:8500"
	}
	if consulScheme == "" {
		consulScheme = "http"
	}

	config := &consul.Config{
		Address: consulAddr,
		Scheme:  consulScheme,
		Token:   consulToken,
	}

	now := time.Now()
	rand.Seed(now.Unix())

	err := Generate(config, serviceNames, oneOffTags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(2)
	}
}

func Generate(config *consul.Config, serviceNames []string, oneOffTags map[string]string) error {
	client, err := consul.NewClient(config)
	if err != nil {
		return err
	}
	catalog := client.Catalog()

	datacenters, err := catalog.Datacenters()
	if err != nil {
		return err
	}

	project := &Project{}

	options := &consul.QueryOptions{}
	for _, dc := range datacenters {
		options.Datacenter = dc
		addressTags := make(map[string]map[string]bool)
		addressName := make(map[string]string)
		for _, serviceName := range serviceNames {
			endpoints, _, err := catalog.Service(serviceName, "", options)
			if err != nil {
				return err
			}

			endpointOrder := rand.Perm(len(endpoints))

			oneOffTag := oneOffTags[serviceName]
			for _, i := range endpointOrder {
				endpoint := endpoints[i]
				address := endpoint.Address
				name := endpoint.Node
				addressName[address] = name
				if _, ok := addressTags[address]; !ok {
					addressTags[address] = make(map[string]bool)
				}
				for _, tag := range endpoint.ServiceTags {
					addressTags[address][tag] = true
				}
				// Add an extra "virtual" tag for the service name.
				addressTags[address][serviceName] = true

				// If we have a one-off tag on this service, tag it as such
				if oneOffTag != "" {
					addressTags[address][oneOffTag] = true
					// Don't do it for the others
					oneOffTag = ""
				}
			}
		}

		for address, tagsMap := range addressTags {
			tags := make([]string, 0, len(tagsMap))
			for tag, _ := range tagsMap {
				tags = append(tags, tag)
			}
			node := Node{
				Name:     addressName[address],
				Hostname: address,
				// TODO: Make username configurable
				Username:       "ubuntu",
				DatacenterName: dc,
				Tags:           strings.Join(tags, ","),
			}
			project.Nodes = append(project.Nodes, node)
		}
	}

	xmlBytes, err := xml.MarshalIndent(project, "", "    ")
	if err != nil {
		return err
	}

	os.Stdout.Write(xmlBytes)
	os.Stdout.Write([]byte{'\n'})

	return nil
}
