package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"text/template"

	"gopkg.in/yaml.v2"
)

var (
	appName        string
	composePath    string
	outputPath     string
	dockerHostConn string
)

const bashTemplate = `#!/bin/bash
/usr/bin/docker {{.DockerHostConnCmdArg}} pull {{.Service.Image}}

if /usr/bin/docker {{.DockerHostConnCmdArg}} ps | grep --quiet {{.Service.Name}}_1 ; then
    /usr/bin/docker {{.DockerHostConnCmdArg}} rm -f {{.Service.Name}}_1
fi

/usr/bin/docker {{.DockerHostConnCmdArg}} run \
  {{if .Service.Privileged}}--privileged=true {{end}} \
  --restart=always \
  -d \
  --name {{.Service.Name}}_1 \
  {{range .Service.Volumes}}-v {{.}} {{end}} \
  {{range .Service.Links}}--link {{.}} {{end}} \
  {{range $key, $value := .Service.Environment}}-e {{$key}}="{{$value}}" {{end}} \
  {{range .Service.Ports}}-p {{.}} {{end}} \
  {{.Service.Image}}  {{.Service.Command}}
`

// ScriptDataTemplate contains the whole data configuration used to fill the script
type ScriptDataTemplate struct {
	AppName              string
	DockerHostConnCmdArg string
	Service              Service
}

// Service has the same structure used by docker-compose.yml
type Service struct {
	Name        string
	Image       string
	Ports       []string
	Volumes     []string
	Links       []string
	Privileged  bool
	Command     string
	Environment map[string]string
}

// Parses the original Yaml to the Service struct
func loadYaml(filename string) (services map[string]Service, err error) {
	data, err := ioutil.ReadFile(filename)
	if err == nil {
		err = yaml.Unmarshal([]byte(data), &services)
	}
	return
}

func setLinksWithAppName(service *Service) {
	for i := range service.Links {
		links := strings.Split(service.Links[i], ":")
		containerName := links[0]
		containerAlias := containerName + "_1"
		if len(links) > 1 {
			containerAlias = links[1]
		}

		service.Links[i] = fmt.Sprintf("%s-%s_1:%s", appName, containerName, containerAlias)
	}
}

func buildScriptDataTemplate(serviceName string, service Service) ScriptDataTemplate {
	// common data template for all services from the same app
	data := ScriptDataTemplate{AppName: appName}
	if dockerHostConn != "" {
		data.DockerHostConnCmdArg = "--host=" + dockerHostConn
	}

	// specific data for each service
	service.Name = appName + "-" + serviceName
	setLinksWithAppName(&service)
	data.Service = service

	return data
}

// Saves the services data into bash scripts
func saveToBash(services map[string]Service) (err error) {
	t := template.New("service-bash-template")
	t, err = t.Parse(bashTemplate)
	if err != nil {
		return err
	}

	for name, service := range services {
		data := buildScriptDataTemplate(name, service)

		f, _ := os.Create(path.Join(outputPath, data.Service.Name+".1.sh"))
		defer f.Close()
		t.Execute(f, data)
	}

	return nil
}

func main() {
	flag.StringVar(&appName, "app", "", "application name")
	flag.StringVar(&composePath, "yml", "docker-compose.yml", "compose file path")
	flag.StringVar(&outputPath, "output", ".", "output directory")
	flag.StringVar(&dockerHostConn, "docker-host", "", "docker host connection")

	flag.Parse()

	if appName == "" {
		fmt.Println("Missing app argument")
		os.Exit(1)
	}

	services, err := loadYaml(composePath)
	if err != nil {
		log.Fatalf("error parsing docker-compose.yml %v", err)
	}

	if err = saveToBash(services); err != nil {
		log.Fatalf("error saving bash template %v", err)
	}

	fmt.Println("Successfully converted Yaml to Bash script.")
}
