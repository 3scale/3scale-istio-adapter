package main

/*
 Utility script to generate deployment template prior to release
*/

import (
	"fmt"
	"log"

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

const configMapName = "3scale-istio-adapter-conf"

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
									Name: "LOG_LEVEL",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "log_level",
										},
									},
								},
								{
									Name: "LOG_JSON",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "log_json",
										},
									},
								},
								{
									Name: "REPORT_METRICS",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "metrics.report",
										},
									},
								},
								{
									Name: "CACHE_TTL_SECONDS",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "system.cache_ttl",
										},
									},
								},
								{
									Name: "CACHE_REFRESH_SECONDS",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "system.cache_ttl",
										},
									},
								},
								{
									Name: "CACHE_ENTRIES_MAX",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "system.cache_max_size",
										},
									},
								},
								{
									Name: "CACHE_REFRESH_RETRIES",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "system.cache_refresh_retries",
										},
									},
								},
								{
									Name: "ALLOW_INSECURE_CONN",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "client.allow_insecure_connections",
										},
									},
								},
								{
									Name: "CLIENT_TIMEOUT_SECONDS",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "client.timeout",
										},
									},
								},
								{
									Name: "GRPC_CONN_MAX_SECONDS",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "grpc.max_conn_timeout",
										},
									},
								},
								{
									Name: "USE_CACHED_BACKEND",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "backend.enable_cache",
										},
									},
								},
								{
									Name: "BACKEND_CACHE_FLUSH_INTERVAL_SECONDS",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "backend.cache_flush_interval",
										},
									},
								},
								{
									Name: "BACKEND_CACHE_POLICY_FAIL_CLOSED",
									ValueFrom: &corev1.EnvVarSource{
										ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: configMapName,
											},
											Key: "backend.policy_fail_closed",
										},
									},
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
