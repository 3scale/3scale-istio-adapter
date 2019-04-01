package main

/*
 Utility script to generate deployment template prior to release
*/

import (
	"fmt"
	"log"
	"strconv"

	"github.com/ghodss/yaml"
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var version string

const resource = "Deployment"
const resourceName = "3scale-istio-adapter"
const selectorKey = "app"
const selectorValue = resourceName

const adapterImageRegistry = "quay.io/3scale/"
const adapterImage = resourceName
const adapterPort = 3333
const metricPort = 8080

const deploymentStrategy = "RollingUpdate"

var replicas int32 = 1
var terminationGraceSeconds int64 = 30

func main() {
	if version == "" {
		log.Fatal("version must be set")
	}
	now := metav1.Now()

	deployment := v1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       resource,
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: resourceName,
			Labels: map[string]string{
				selectorKey: selectorValue,
			},
			CreationTimestamp: now,
		},
		Spec: v1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					selectorKey: selectorValue,
				},
			},
			Strategy: v1.DeploymentStrategy{
				Type: v1.DeploymentStrategyType(deploymentStrategy),
				RollingUpdate: &v1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{
						IntVal: 25,
					},
					MaxSurge: &intstr.IntOrString{
						IntVal: 25,
					},
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						selectorKey: selectorValue,
					},
					CreationTimestamp: now,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:            resourceName,
							Image:           adapterImageRegistry + adapterImage + ":" + version,
							ImagePullPolicy: corev1.PullAlways,
							Ports: []corev1.ContainerPort{
								{
									Name:          "adapter",
									ContainerPort: adapterPort,
									Protocol:      corev1.ProtocolTCP,
								},
								{
									Name:          "prometheus",
									ContainerPort: metricPort,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							LivenessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.IntOrString{
											IntVal: adapterPort,
										},
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
							},
							Env: []corev1.EnvVar{
								{
									Name:  "THREESCALE_LOG_JSON",
									Value: "true",
								},
								{
									Name:  "THREESCALE_REPORT_METRICS",
									Value: "true",
								},
								{
									Name:  "THREESCALE_METRICS_PORT",
									Value: strconv.Itoa(metricPort),
								},
							},
							Resources:              corev1.ResourceRequirements{},
							TerminationMessagePath: "/dev/termination-log",
						},
					},
					DNSPolicy:                     corev1.DNSClusterFirst,
					RestartPolicy:                 corev1.RestartPolicyAlways,
					SecurityContext:               &corev1.PodSecurityContext{},
					TerminationGracePeriodSeconds: &terminationGraceSeconds,
				},
			},
		},
	}

	b, err := yaml.Marshal(deployment)
	if err != nil {
		panic("unexpected marshal error" + err.Error())
	}
	fmt.Printf("# This code was generated as part of the release process using make release for version %s\n%s", version, string(b))
}
