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
	uid           string
	outputTo      string
	authType      int
	fixup         bool

	version string
)

const (
	tokenDescription      = "3scale access token (required)"
	svcIDDescription      = "The ID of the 3scale service (required)"
	uidDescription        = "Unique UID for the handler, if you don't want it derived from the URL (optional)"
	threescaleDescription = "The 3scale admin portal URL (required)"
	outputDescription     = "File to output templates. Prints to stdout if none provided"
	authTypeDescription   = "3scale authentication pattern to use. 1=ApiKey, 2=AppID. Default template supports both if none provided"
	fixupDescription      = "Try to automatically fix validation errors"

	outputDefault, tokenDefault, svcDefault, urlDefault = "", "", "", ""
)

func init() {
	flag.StringVar(&accessToken, "token", tokenDefault, tokenDescription)
	flag.StringVar(&accessToken, "t", tokenDefault, tokenDescription+" (short)")

	flag.StringVar(&svcID, "service", svcDefault, svcIDDescription)
	flag.StringVar(&svcID, "s", svcDefault, svcIDDescription+" (short)")

	flag.StringVar(&uid, "uid", "", uidDescription)

	flag.StringVar(&threescaleURL, "url", urlDefault, threescaleDescription)
	flag.StringVar(&threescaleURL, "u", urlDefault, threescaleDescription+" (short)")

	flag.StringVar(&outputTo, "output", outputDefault, outputDescription)
	flag.StringVar(&outputTo, "o", outputDefault, outputDescription+" (short)")

	flag.IntVar(&authType, "auth", 0, authTypeDescription)

	flag.BoolVar(&fixup, "fixup", false, fixupDescription)

	v := flag.Bool("v", false, "Prints CLI version")

	flag.Parse()
	if *v {
		if version == "" {
			version = "undefined"
		}
		log.Printf("3scale-config-gen version is %s", version)
		os.Exit(0)
	}

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

	if threescaleURL == "" {
		errs = append(errs, errors.New("error missing parameter. --url is required"))
	}

	if svcID == "" {
		errs = append(errs, errors.New("error missing parameter. --service is required"))
	}

	return errs
}

func execute() []error {
	var errs []error
	var writeTo io.Writer

	handler, errList := templating.NewHandler(accessToken, threescaleURL, svcID, fixup)
	if len(errList) > 0 {
		errs = append(errs, errList...)
	}

	instance := templating.GetDefaultInstance()
	instance.AuthnMethod = templating.AuthenticationMethod(authType)

	if uid == "" {
		// generate a UID from the URL
		url, err := templating.ParseURL(handler.SystemURL)
		if err != nil {
			errs = append(errs, fmt.Errorf("URL parsing for UID generation failed: %s", err.Error()))
		} else {
			uid, err = templating.UidGenerator(url, handler.ServiceID)
			if err != nil {
				errs = append(errs, fmt.Errorf("UID generation failed: %s", err.Error()))
			}
		}
	}

	cg, errList := templating.NewConfigGenerator(*handler, instance, uid, fixup)
	if len(errList) > 0 {
		errs = append(errs, errList...)
	}

	if len(errs) > 0 {
		return errs
	}

	cg.Rule.Conditions = append(cg.Rule.Conditions, cg.GetDefaultMatchConditions()...)

	if outputTo == "" {
		writeTo = os.Stdout
	} else {
		f, err := os.Create(outputTo)
		if err != nil {
			panic(err)
		}
		writeTo = f
	}

	err := cg.OutputAll(writeTo)
	if err != nil {
		return []error{err}
	}

	err = cg.OutputUID(writeTo)
	if err != nil {
		return []error{err}
	}

	return nil
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

	errs = execute()
	if errs != nil {
		for _, i := range errs {
			fmt.Println(i.Error())
		}
		os.Exit(1)
	}
}
