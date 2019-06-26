package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/3scale/3scale-istio-adapter/pkg/kubernetes"
)

var (
	accessToken   string
	svcID         string
	threescaleURL string
	backendURL    string
	name          string
	outputTo      string
	authType      int
	namespace     string

	version string
)

const (
	nameDescription       = "Unique name for this (url,token) pair (required)"
	tokenDescription      = "3scale access token (required)"
	threescaleDescription = "The 3scale admin portal URL (required)"
	backendDescription    = "The 3scale backend url"

	svcIDDescription     = "The ID of the 3scale service. If set the generated configuration will apply to this service only."
	outputDescription    = "File to output templates. Prints to stdout if none provided"
	authTypeDescription  = "3scale authentication pattern to use. 1=ApiKey, 2=AppID, 3=OpenID Connect. Default template supports a hybrid if none provided"
	namespaceDescription = "The namespace which the manifests should be generated for. Default 'istio-system'"

	outputDefault, tokenDefault, svcDefault, urlDefault = "", "", "", ""

	istioNamespaceDefault = kubernetes.DefaultNamespace
)

func init() {
	flag.StringVar(&accessToken, "token", tokenDefault, tokenDescription)
	flag.StringVar(&accessToken, "t", tokenDefault, tokenDescription+" (short)")

	flag.StringVar(&svcID, "service", svcDefault, svcIDDescription)

	flag.StringVar(&name, "name", "", nameDescription)

	flag.StringVar(&threescaleURL, "url", urlDefault, threescaleDescription)
	flag.StringVar(&threescaleURL, "u", urlDefault, threescaleDescription+" (short)")

	flag.StringVar(&backendURL, "backend-url", urlDefault, backendDescription)

	flag.StringVar(&outputTo, "output", outputDefault, outputDescription)
	flag.StringVar(&outputTo, "o", outputDefault, outputDescription+" (short)")

	flag.IntVar(&authType, "auth", 0, authTypeDescription)

	flag.StringVar(&namespace, "namespace", istioNamespaceDefault, namespaceDescription)
	flag.StringVar(&namespace, "n", istioNamespaceDefault, namespaceDescription+" (short)")

	v := flag.Bool("version", false, "Prints CLI version")

	flag.Parse()
	if *v {
		if version == "" {
			version = "undefined"
		}
		fmt.Printf("3scale-config-gen version %s\n", version)
		os.Exit(0)
	}

	checkEnv()
}

func checkEnv() {
	if accessToken == "" {
		accessToken = os.Getenv("THREESCALE_ACCESS_TOKEN")
	}

	if threescaleURL == "" {
		threescaleURL = os.Getenv("THREESCALE_ADMIN_PORTAL")
	}
}

func validate() []error {
	var errs []error
	if name == "" {
		errs = append(errs, errors.New("error missing parameter. --name is required"))
	}

	if accessToken == "" {
		errs = append(errs, errors.New("error missing parameter. --token is required"))
	}

	if threescaleURL == "" {
		errs = append(errs, errors.New("error missing parameter. --url is required"))
	}

	return errs
}

func execute() error {
	var writeTo io.Writer

	handler, err := kubernetes.NewThreescaleHandlerSpec(accessToken, threescaleURL, svcID)
	if err != nil {
		panic("error creating required handler " + err.Error())
	}

	// set the optional backend url override
	handler.Params.BackendUrl = backendURL

	var instance *kubernetes.BaseInstance
	switch authType {
	case 0:
		instance = kubernetes.NewDefaultHybridInstance()
	case 1:
		instance = kubernetes.NewApiKeyInstance(kubernetes.DefaultApiKeyAttribute)
	case 2:
		instance = kubernetes.NewAppIDAppKeyInstance(kubernetes.DefaultAppIDAttribute, kubernetes.DefaultAppKeyAttribute)
	case 3:
		instance = kubernetes.NewOIDCInstance(kubernetes.DefaultOIDCAttribute, kubernetes.DefaultAppKeyAttribute)
	default:
		return fmt.Errorf("unsupported authentication type provided")

	}

	handlerName := fmt.Sprintf("%s.handler.%s", name, namespace)
	instanceName := fmt.Sprintf("%s.instance.%s", name, namespace)
	rule := kubernetes.NewRule(kubernetes.GetDefaultMatchConditions(name), handlerName, instanceName)

	cg, err := kubernetes.NewConfigGenerator(name, *handler, *instance, rule)
	if err != nil {
		panic("error creating config generator " + err.Error())
	}

	cg.SetNamespace(namespace)

	if outputTo == "" {
		writeTo = os.Stdout
	} else {
		f, err := os.Create(outputTo)
		if err != nil {
			panic(err)
		}
		writeTo = f
	}

	return cg.OutputAll(writeTo)
}

func main() {
	errs := validate()
	if errs != nil {
		log.Println("Error validating input:")
		for _, i := range errs {
			fmt.Println(i.Error())
		}
		os.Exit(1)
	}

	err := execute()
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}
