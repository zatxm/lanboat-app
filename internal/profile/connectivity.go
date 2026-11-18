package profile

var SpeedURLs = []string{"https://speed.cloudflare.com", "https://speedtest.net", "https://ipcheck.ing", "https://web1.cachefly.net/speedtest/index.html"}
var TestURLs = []string{"https://www.gstatic.com/generate_204", "http://www.msftconnecttest.com/connecttest.txt", "http://cp.cloudflare.com/generate_204", "https://checkip.amazonaws.com", "http://connectivity-check.ubuntu.com", "http://detectportal.firefox.com/success.txt"}

func (s Settings) SpeedTarget() string {
	if s.SpeedURL != "" {
		return s.SpeedURL
	}
	return SpeedURLs[0]
}
func (s Settings) TestTarget() string {
	if s.TestURL != "" {
		return s.TestURL
	}
	return TestURLs[0]
}
func (s Settings) TestSeconds() int {
	if s.TestTimeout != 0 {
		return s.TestTimeout
	}
	return 15
}
func (s Settings) TestWorkers() int {
	if s.TestConcurrency != 0 {
		return s.TestConcurrency
	}
	return 20
}
