package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/3scale/3scale-istio-adapter/pkg/templating"
)

var (
	accessToken   string
	svcID         string
	threescaleURL string
	outputTo      string
	authType      int
)

const (
	tokenDescription      = "3scale access token (required)"
	svcIDDescription      = "The ID of the 3scale service (required)"
	threescaleDescription = "The 3scale admin portal URL (required)"
	outputDescription     = "File to output templates. Prints to stdout if none provided"
	authTypeDescription   = "3scale authentication pattern to use. 1=ApiKey, 2=AppID. Default template supports both if none provided"

	outputDefault, tokenDefault, svcDefault, urlDefault = "", "", "", ""
)

func init() {
	flag.StringVar(&accessToken, "token", tokenDefault, tokenDescription)
	flag.StringVar(&accessToken, "t", tokenDefault, tokenDescription+" (short)")

	flag.StringVar(&svcID, "service", svcDefault, svcIDDescription)
	flag.StringVar(&svcID, "s", svcDefault, svcIDDescription+" (short)")

	flag.StringVar(&threescaleURL, "url", urlDefault, threescaleDescription)
	flag.StringVar(&threescaleURL, "u", urlDefault, threescaleDescription+" (short)")

	flag.StringVar(&outputTo, "output", outputDefault, outputDescription)
	flag.StringVar(&outputTo, "o", outputDefault, outputDescription+" (short)")

	flag.IntVar(&authType, "auth", 0, authTypeDescription)

	flag.Parse()

	checkEnv()
}

func checkEnv() {
	if accessToken == "" {
		accessToken = os.Getenv("THREESCALE_ACCESS_TOKEN")
	}

	if svcID == "" {
		svcID = os.Getenv("THREESCALE_SERVICE_ID")
	}

	if threescaleURL == "" {
		threescaleURL = os.Getenv("THREESCALE_ADMIN_PORTAL")
	}
}

func validate() []error {
	var errs []error
	if accessToken == "" {
		errs = append(errs, errors.New("error missing parameter. --token is required"))
	}

	if svcID == "" {
		errs = append(errs, errors.New("error missing parameter. --service is required"))
	}

	if threescaleURL == "" {
		errs = append(errs, errors.New("error missing parameter. --url is required"))
	}
	return errs
}

func execute() error {
	var writeTo io.Writer

	handler := templating.Handler{
		AccessToken: accessToken,
		SystemURL:   threescaleURL,
		ServiceID:   svcID,
	}

	instance := templating.GetDefaultInstance()
	instance.AuthnMethod = templating.AuthenticationMethod(authType)

	cg, err := templating.NewConfigGenerator(handler, instance, templating.Rule{})
	if err != nil {
		return fmt.Errorf("error - generating configuration - %s", err.Error())
	}

	cg.PopulateDefaultRules()

	if outputTo == "" {
		writeTo = os.Stdout
	} else {
		f, err := os.Create(outputTo)
		if err != nil {
			panic(err)
		}
		writeTo = f
	}

	err = cg.OutputAll(writeTo)
	if err != nil {
		return err
	}

	return cg.OutputUID(writeTo)
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
		log.Printf(err.Error())
		os.Exit(1)
	}
}
