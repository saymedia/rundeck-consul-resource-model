package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"strings"

	consul "github.com/hashicorp/consul/api"
)

type Project struct {
	XMLName xml.Name `xml:"project"`
	Nodes   []Node
}

type Node struct {
	XMLName        xml.Name `xml:"node"`
	Name           string   `xml:"name,attr"`
	Hostname       string   `xml:"hostname,att"`
	Tags           string   `xml:"tags,attr,omitempty"`
	Username       string   `xml:"username,attr"`
	DatacenterName string   `xml:"datacenter,attr"`
}

func main() {

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s service-names...\n", os.Args[0])
		os.Exit(1)
	}

	serviceNames := os.Args[1:]
	consulAddr := os.Getenv("CONSUL_ADDRESS")
	consulScheme := os.Getenv("CONSUL_SCHEME")
	consulToken := os.Getenv("CONSUL_TOKEN")

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

	err := Generate(config, serviceNames)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(2)
	}
}

func Generate(config *consul.Config, serviceNames []string) error {
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
		for _, serviceName := range serviceNames {
			endpoints, _, err := catalog.Service(serviceName, "", options)
			if err != nil {
				return err
			}

			for _, endpoint := range endpoints {
				address := endpoint.Address
				if _, ok := addressTags[address]; !ok {
					addressTags[address] = make(map[string]bool)
				}
				for _, tag := range endpoint.ServiceTags {
					addressTags[address][tag] = true
				}
				// Add an extra "virtual" tag for the service name.
				addressTags[address][serviceName] = true
			}
		}

		for address, tagsMap := range addressTags {
			tags := make([]string, 0, len(tagsMap))
			for tag, _ := range tagsMap {
				tags = append(tags, tag)
			}
			node := Node{
				Name:     address,
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
