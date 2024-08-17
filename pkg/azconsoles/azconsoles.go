package azconsoles

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	_ "time"
	"github.com/google/uuid"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"

	armruntime "github.com/Azure/azure-sdk-for-go/sdk/azcore/arm/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/serialconsole/armserialconsole"
)

// nopReadSeekCloser wraps an io.Reader to provide io.ReadSeekCloser
type nopReadSeekCloser struct {
	io.ReadSeeker
}

func (nopReadSeekCloser) Close() error { return nil }

// NewNopReadSeekCloser converts an io.Reader into an io.ReadSeekCloser
func NewNopReadSeekCloser(r io.Reader) io.ReadSeekCloser {
	return nopReadSeekCloser{ReadSeeker: bytes.NewReader(r.(*bytes.Buffer).Bytes())}
}

func SendReset(token string, connURL string) {
	adminURL := strings.Replace(connURL, "/client", "/adminCommand/reset", 1)
	adminURL = strings.Replace(adminURL, "wss://", "https://", 1)
	uuid := uuid.New()
	sysRq := fmt.Sprintf(`{"command":"reset", "requestId": "%v", "commandParameters": {}}`, uuid.String())

	req, err := http.NewRequest("POST", adminURL, bytes.NewBuffer([]byte(sysRq)))
	if err != nil {
		fmt.Println(err)
		return
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %v", token))
	req.Close = true

	client := &http.Client{}
	resp, err := client.Do(req)

	if err != nil {
		fmt.Println(err)
	} else {
		defer resp.Body.Close()
		_, _ = ioutil.ReadAll(resp.Body)
	}
}

func StartSerialConsole(subscriptionID string, resourceGroupName string, virtualMachineName string) (net.Conn, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	// It looks like this SDK is broken because it is generated based on our
	// busted swagger specs which are incorrect
	// ---
	// So here's the nasty ugly way of doing it, rather than a nice
	// armserialclient.NewClient(...).Create(...)
	vmResourceID := fmt.Sprintf(
		"/subscriptions/%v/resourcegroups/%v/providers/Microsoft.Compute/%v/%v",
		subscriptionID,
		resourceGroupName,
		"virtualMachines",
		virtualMachineName)

	serialPortResource := fmt.Sprintf(
		"/providers/%v/serialPorts/%v/connect",
		"Microsoft.SerialConsole",
		"0")

	armURL := "https://management.azure.com"

	pipeline, err := armruntime.NewPipeline("module", "version", cred,
		runtime.PipelineOptions{}, nil)
	if err != nil {
		return nil, err
	}

	endpoint := armURL + vmResourceID + serialPortResource
	req, err := runtime.NewRequest(context.Background(), http.MethodPost,
		endpoint)
	if err != nil {
		return nil, err
	}

	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", "2018-05-01")
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}
	state := armserialconsole.SerialPortStateEnabled
	err = runtime.MarshalAsJSON(req, armserialconsole.SerialPort{
		Properties: &armserialconsole.SerialPortProperties{
			State: &state,
		},
	})

	if err != nil {
		return nil, err
	}

	res, err := pipeline.Do(req)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 4096)
	n, err := res.Body.Read(buf)
	if err != nil {
		return nil, err
	}

	type ConnectResponse struct {
		ConnectionString string `json:"connectionString"`
	}

	connRes := ConnectResponse{}
	err = json.Unmarshal(buf[:n], &connRes)
	if err != nil {
		return nil, err
	}

	if connRes.ConnectionString == "" {
		return nil, fmt.Errorf("empty connection string")
	}

	token := strings.TrimPrefix(res.Request.Header.Get("Authorization"), "Bearer ")
	url := connRes.ConnectionString

	wsCtx := context.Background()
	conn, _, _, err := ws.Dial(wsCtx, url)
	if err != nil {
		return nil, err
	}

	_, err = wsutil.ReadServerText(conn)
	if err != nil {
		return nil, err
	}

	wsutil.WriteClientText(conn, []byte(token))

	return conn, nil
}

func ConnectCloudShell() (net.Conn, error) {
	endpoint := "https://management.azure.com/providers/Microsoft.Portal/consoles/default"

	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, err
	}

	pipeline, err := armruntime.NewPipeline("module", "version", cred,
		runtime.PipelineOptions{}, nil)
	if err != nil {
		return nil, err
	}

	req, err := runtime.NewRequest(context.Background(), http.MethodPut, endpoint)
	if err != nil {
		return nil, err
	}

	type CloudShellRequestProperties struct {
		OSType string `json:"osType"`
	}

	type CloudShellRequest struct {
		Properties CloudShellRequestProperties `json:"properties"`
	}

	type CloudShellResponseProperties struct {
		OSType string `json:"osType"`
		ProvisioningState string `json:"provisioningState"`
		URI string `json:"uri"`
	}

	type CloudShellResponse struct {
		Properties CloudShellResponseProperties `json:"properties"`
	}

	csr := CloudShellRequest{
		Properties: CloudShellRequestProperties{
			OSType: "linux",
		},
	}

	reqQP := req.Raw().URL.Query()
	reqQP.Set("api-version", "2023-02-01-preview")
	req.Raw().URL.RawQuery = reqQP.Encode()
	req.Raw().Header["Accept"] = []string{"application/json"}

	jsonBody, err := json.Marshal(&csr)
	if err != nil {
		return nil, err
	}

	body := bytes.NewBuffer([]byte(jsonBody))
	req.SetBody(NewNopReadSeekCloser(body), "application/json")

	res, err := pipeline.Do(req)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 4096)
	n, err := res.Body.Read(buf)
	if err != nil {
		return nil, err
	}

	connRes := CloudShellResponse{}
	err = json.Unmarshal(buf[:n], &connRes)
	if err != nil {
		return nil, err
	}

	token := strings.TrimPrefix(res.Request.Header.Get("Authorization"), "Bearer ")
	type CloudShellWebsocketResponse struct {
		ID string `json:"id"`
		SocketURI string `json:"socketUri"`
		IdleTimeout string `json:"idleTimeout"`
		TokenUpdated bool `json:"tokenUpdated"`
		RootDirectory string `json:"rootDirectory"`
	}

	url := connRes.Properties.URI + "/terminals?cols=80&rows=24&version=2019-01-01&shell=bash"
	state := connRes.Properties.ProvisioningState
	if state != "Succeeded" {
		return nil, fmt.Errorf("invalid provisioningState: %v", state)
	}

	fmt.Println("Succeeded provisioning state - uri: ", url)

	wsPipeline, err := armruntime.NewPipeline("module", "version", cred,
		runtime.PipelineOptions{}, nil)
	if err != nil {
		return nil, err
	}

	req, err = runtime.NewRequest(context.Background(), http.MethodPost, url)
	if err != nil {
		return nil, err
	}
	req.SetBody(NewNopReadSeekCloser(bytes.NewBuffer([]byte(string("{}")))), "application/json")
	req.Raw().Header["Accept"] = []string{"application/json"}
	req.Raw().Header["Referer"] = []string{"https://ux.console.azure.com/"}
	res, err = wsPipeline.Do(req)
	if err != nil {
		return nil, err
	}

	wsBuf := make([]byte, 4096)
	n, err = res.Body.Read(wsBuf)
	if err != nil {
		return nil, err
	}

	fmt.Printf("%v\n", string(wsBuf))

	wsRes := CloudShellWebsocketResponse{}
	err = json.Unmarshal(wsBuf[:n], &wsRes)
	if err != nil {
		return nil, err
	}

	if wsRes.SocketURI == "" {
		return nil, fmt.Errorf("empty websocket uri: %v", wsRes)
	}

	if wsRes.TokenUpdated {
		token = strings.TrimPrefix(res.Request.Header.Get("Authorization"), "Bearer ")
	}

	id := wsRes.ID
	newURL := strings.Replace(strings.Replace(fmt.Sprintf("%v/terminals/%v", connRes.Properties.URI, id), ":443", "/$hc", 1), "https://", "wss://", 1)

	wsCtx := context.Background()
	wsCtrlCtx := context.Background()
	conn, _, _, err := ws.Dial(wsCtx, newURL)
	if err != nil {
		_ = token
		return nil, err
	}


	ctrlConn, _, _, err := ws.Dial(wsCtrlCtx, newURL + "/control")
	if err != nil {
		return nil, err
	}

	_ = ctrlConn
	wsutil.WriteClientText(conn, []byte(""))
	ctrlText, err := wsutil.ReadServerText(conn)
	if err != nil {
	 	return nil, err
	}

	fmt.Printf("%v\n", string(ctrlText))
	return conn, nil
}
