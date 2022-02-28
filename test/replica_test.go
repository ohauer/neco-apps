package test

import (
	"encoding/json"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
)

// checkDeploymentReplicas checks the number of available replicas and updated replicas for a Deployment.
// If desiredReplicas is less than zero, `.spec.replicas` is used instead.
func checkDeploymentReplicas(name, namespace string, desiredReplicas int) error {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "deployment", "-n", namespace, name, "-o", "json")
	if err != nil {
		return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	}

	deployment := new(appsv1.Deployment)
	err = json.Unmarshal(stdout, deployment)
	if err != nil {
		return err
	}

	if desiredReplicas < 0 {
		if deployment.Spec.Replicas == nil {
			desiredReplicas = 1
		} else {
			desiredReplicas = int(*deployment.Spec.Replicas)
		}
	}

	if int(deployment.Status.AvailableReplicas) != desiredReplicas {
		return fmt.Errorf("AvailableReplicas of Deployment %s/%s is not %d: %d", namespace, name, desiredReplicas, deployment.Status.AvailableReplicas)
	}
	if int(deployment.Status.UpdatedReplicas) != desiredReplicas {
		return fmt.Errorf("UpdatedReplicas of Deployment %s/%s is not %d: %d", namespace, name, desiredReplicas, deployment.Status.UpdatedReplicas)
	}

	return nil
}

// checkStatefulSetReplicas checks the number of available replicas and updated replicas for a StatefulSet.
// If desiredReplicas is less than zero, `.spec.replicas` is used instead.
func checkStatefulSetReplicas(name, namespace string, desiredReplicas int) error {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "statefulset", "-n", namespace, name, "-o", "json")
	if err != nil {
		return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	}

	sts := new(appsv1.StatefulSet)
	err = json.Unmarshal(stdout, sts)
	if err != nil {
		return err
	}

	if desiredReplicas < 0 {
		if sts.Spec.Replicas == nil {
			desiredReplicas = 1
		} else {
			desiredReplicas = int(*sts.Spec.Replicas)
		}
	}

	if int(sts.Status.AvailableReplicas) != desiredReplicas {
		return fmt.Errorf("AvailableReplicas of StatefulSet %s/%s is not %d: %d", namespace, name, desiredReplicas, sts.Status.AvailableReplicas)
	}
	if int(sts.Status.UpdatedReplicas) != desiredReplicas {
		return fmt.Errorf("UpdatedReplicas of StatefulSet %s/%s is not %d: %d", namespace, name, desiredReplicas, sts.Status.UpdatedReplicas)
	}

	return nil
}

// checkDaemonSetNumber checks the number of available nodes and updated nodes for a DaemonSet.
// If desiredNumber is less than zero, `.status.desiredNumberScheduled` is used instead.
// Note: DaemonSets which run no Pods are not supported.
func checkDaemonSetNumber(name, namespace string, desiredNumber int) error {
	stdout, stderr, err := ExecAt(boot0, "kubectl", "get", "daemonset", "-n", namespace, name, "-o", "json")
	if err != nil {
		return fmt.Errorf("stdout: %s, stderr: %s, err: %v", stdout, stderr, err)
	}

	ds := new(appsv1.DaemonSet)
	err = json.Unmarshal(stdout, ds)
	if err != nil {
		return err
	}

	if desiredNumber < 0 {
		desiredNumber = int(ds.Status.DesiredNumberScheduled)
		if desiredNumber <= 0 {
			return fmt.Errorf("DesiredNumberScheduled of DaemonSet %s/%s is not updated yet", namespace, name)
		}
	}

	if int(ds.Status.NumberAvailable) != desiredNumber {
		return fmt.Errorf("NumberAvailable of DaemonSet %s/%s is not %d: %d", namespace, name, desiredNumber, ds.Status.NumberAvailable)
	}
	if int(ds.Status.UpdatedNumberScheduled) != desiredNumber {
		return fmt.Errorf("UpdatedNumberScheduled of DaemonSet %s/%s is not %d: %d", namespace, name, desiredNumber, ds.Status.UpdatedNumberScheduled)
	}

	return nil
}
