package tests

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aiven/aiven-operator/api/v1alpha1"
)

func getClickhouseUserYaml(project, chName, userName string) string {
	return fmt.Sprintf(`
apiVersion: aiven.io/v1alpha1
kind: Clickhouse
metadata:
  name: %[2]s
spec:
  authSecretRef:
    name: aiven-token
    key: token

  project: %[1]s
  cloudName: google-europe-west1
  plan: startup-16

---

apiVersion: aiven.io/v1alpha1
kind: ClickhouseUser
metadata:
  name: %[3]s
spec:
  authSecretRef:
    name: aiven-token
    key: token

  project: %[1]s
  serviceName: %[2]s

`, project, chName, userName)
}

func TestClickhouseUser(t *testing.T) {
	t.Parallel()
	defer recoverPanic(t)

	// GIVEN
	chName := randName("clickhouse-user")
	userName := randName("clickhouse-user")
	yml := getClickhouseUserYaml(testProject, chName, userName)
	s, err := NewSession(k8sClient, avnClient, testProject, yml)
	require.NoError(t, err)

	// Cleans test afterwards
	defer s.Destroy()

	// WHEN
	// Applies given manifest
	require.NoError(t, s.Apply())

	// Waits kube objects
	ch := new(v1alpha1.Clickhouse)
	require.NoError(t, s.GetRunning(ch, chName))

	// THEN
	chAvn, err := avnClient.Services.Get(testProject, chName)
	require.NoError(t, err)
	assert.Equal(t, chAvn.Name, ch.GetName())
	assert.Equal(t, "RUNNING", ch.Status.State)
	assert.Equal(t, chAvn.State, ch.Status.State)
	assert.Equal(t, chAvn.Plan, ch.Spec.Plan)
	assert.Equal(t, chAvn.CloudName, ch.Spec.CloudName)

	user := new(v1alpha1.ClickhouseUser)
	require.NoError(t, s.GetRunning(user, userName))

	userAvn, err := avnClient.ClickhouseUser.Get(testProject, chName, user.Status.UUID)
	require.NoError(t, err)
	assert.Equal(t, userName, user.GetName())
	assert.Equal(t, userAvn.Name, user.GetName())

	ctx := context.Background()
	secret := new(corev1.Secret)
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: userName, Namespace: "default"}, secret))
	assert.NotEmpty(t, secret.Data["HOST"])
	assert.NotEmpty(t, secret.Data["PORT"])
	assert.NotEmpty(t, secret.Data["PASSWORD"])
	assert.NotEmpty(t, secret.Data["USERNAME"])

	// We need to validate deletion,
	// because we can get false positive here:
	// if service is deleted, user is destroyed in Aiven. No service — no user. No user — no user.
	// And we make sure that controller can delete user itself
	assert.NoError(t, s.Delete(user, func() error {
		_, err = avnClient.ClickhouseUser.Get(testProject, chName, user.Status.UUID)
		return err
	}))
}
