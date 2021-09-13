package cautils

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"

	"github.com/armosec/kubescape/cautils/getter"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/armosec/kubescape/cautils/k8sinterface"
	corev1 "k8s.io/api/core/v1"
)

const (
	configMapName  = "kubescape"
	configFileName = "config"
)

type ConfigObj struct {
	CustomerGUID string `json:"customerGUID"`
	Token        string `json:"token"`
}

func (co *ConfigObj) Json() []byte {
	if b, err := json.Marshal(co); err == nil {
		return b
	}
	return []byte{}
}

type IClusterConfig interface {
	SetCustomerGUID() error
	GetCustomerGUID() string
	GenerateURL()
}

type ClusterConfig struct {
	k8s       *k8sinterface.KubernetesApi
	defaultNS string
	armoAPI   *getter.ArmoAPI
	configObj *ConfigObj
}

type EmptyConfig struct {
}

func (c *EmptyConfig) GenerateURL() {
}

func (c *EmptyConfig) SetCustomerGUID() error {
	return nil
}

func (c *EmptyConfig) GetCustomerGUID() string {
	return ""
}

func NewEmptyConfig() *EmptyConfig {
	return &EmptyConfig{}
}

func NewClusterConfig(k8s *k8sinterface.KubernetesApi, armoAPI *getter.ArmoAPI) *ClusterConfig {
	return &ClusterConfig{
		k8s:       k8s,
		armoAPI:   armoAPI,
		defaultNS: k8sinterface.GetDefaultNamespace(),
	}
}

func (c *ClusterConfig) update(configObj *ConfigObj) {
	c.configObj = configObj
	ioutil.WriteFile(getter.GetDefaultPath(configFileName+".json"), c.configObj.Json(), 0664)
}
func (c *ClusterConfig) GenerateURL() {
	u := url.URL{}
	u.Scheme = "https"
	u.Host = getter.ArmoFEURL
	u.Path = "account/sign-up"
	q := u.Query()
	q.Add("invitationToken", c.configObj.Token)
	q.Add("customerGUID", c.configObj.CustomerGUID)

	u.RawQuery = q.Encode()
	fmt.Println("To view all controls and get remediations visit:")
	InfoTextDisplay(os.Stdout, u.String()+"\n")

}

func (c *ClusterConfig) GetCustomerGUID() string {
	return c.configObj.CustomerGUID
}
func (c *ClusterConfig) SetCustomerGUID() error {

	// get from configMap
	if configObj, _ := c.loadConfigFromConfigMap(); configObj != nil {
		c.update(configObj)
		return nil
	}

	// get from file
	if configObj, _ := c.loadConfigFromFile(); configObj != nil {
		c.update(configObj)
		c.updateConfigMap()
		return nil
	}

	// get from armoBE
	if tenantResponse, err := c.armoAPI.GetCustomerGUID(); tenantResponse != nil {
		c.update(&ConfigObj{CustomerGUID: tenantResponse.TenantID, Token: tenantResponse.Token})
		return c.updateConfigMap()
	} else {
		return err
	}
}

func (c *ClusterConfig) loadConfigFromConfigMap() (*ConfigObj, error) {
	if c.k8s == nil {
		return nil, nil
	}
	configMap, err := c.k8s.KubernetesClient.CoreV1().ConfigMaps(c.defaultNS).Get(context.Background(), configMapName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}

	if bData, err := json.Marshal(configMap.Data); err == nil {
		return readConfig(bData)
	}
	return nil, nil
}

func (c *ClusterConfig) updateConfigMap() error {
	if c.k8s == nil {
		return nil
	}
	configMap, err := c.k8s.KubernetesClient.CoreV1().ConfigMaps(c.defaultNS).Get(context.Background(), configMapName, metav1.GetOptions{})
	if err != nil {
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name: configMapName,
			},
		}
	}

	c.updateConfigData(configMap)

	if err != nil {
		_, err = c.k8s.KubernetesClient.CoreV1().ConfigMaps(c.defaultNS).Create(context.Background(), configMap, metav1.CreateOptions{})
	} else {
		_, err = c.k8s.KubernetesClient.CoreV1().ConfigMaps(configMap.Namespace).Update(context.Background(), configMap, metav1.UpdateOptions{})
	}
	return err
}
func (c *ClusterConfig) updateConfigData(configMap *corev1.ConfigMap) {
	if len(configMap.Data) == 0 {
		configMap.Data = make(map[string]string)
	}
	m := c.ToMapString()
	for k, v := range m {
		if s, ok := v.(string); ok {
			configMap.Data[k] = s
		}
	}
}
func (c *ClusterConfig) loadConfigFromFile() (*ConfigObj, error) {
	dat, err := ioutil.ReadFile(getter.GetDefaultPath(configFileName + ".json"))
	if err != nil {
		return nil, err
	}

	return readConfig(dat)
}
func readConfig(dat []byte) (*ConfigObj, error) {

	if len(dat) == 0 {
		return nil, nil
	}
	configObj := &ConfigObj{}
	err := json.Unmarshal(dat, configObj)

	return configObj, err
}
func (c *ClusterConfig) ToMapString() map[string]interface{} {
	m := map[string]interface{}{}
	bc, _ := json.Marshal(c.configObj)
	json.Unmarshal(bc, &m)
	return m
}