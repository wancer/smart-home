package event

type Status2 struct {
	StatusFWR FirmwareEvent `json:"StatusFWR"`
}

type FirmwareEvent struct {
	Version       string
	BuildDateTime TasmotaTime
	Boot          int
	Core          string
	SDK           string
	CpuFrequency  int
	Hardware      string
	CR            string
}
