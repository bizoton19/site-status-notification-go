package main

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/sendgrid/sendgrid-go"
	mail "github.com/sendgrid/sendgrid-go/helpers/mail"
	"github.com/site-status-notification/site-status-notification-go/cmd/poll-worker/config"
	"github.com/spf13/viper"
)

const (
	//each go routine will be have a task to poll on specified intervals
	numPollers      = 1
	pollIntervall   = 15 * time.Minute
	statusIntervall = 30 * time.Minute
	errTimeout      = 10 * time.Second
)

//GetURL will get the formatted url
func GetIncidentDataURL(page string) string {
	var safeURL = url.QueryEscape(page)
	var incidentDataURL = "" + safeURL

	return incidentDataURL

}

var urlsToPoll = []string{
	"https://www.saferproducts.gov/",
	"https://www.saferproducts.gov/CPSRMSPublic/Incidents/ReportIncident.aspx",
	"https://www.saferproducts.gov/Search/",
	"https://www.saferproducts.gov/CPSRMSPublic/Industry/Home.aspx",
	"https://www.cpsc.gov",
	"https://onsafety.cpsc.gov",
	"http://www.atvsafety.gov/",
	"https://www.anchorit.gov/",
	"https://search.cpsc.gov/",
	"https://www.poolsafely.gov",
	"https://cpscnet.cpsc.gov/pin/",
	"https://cpscnet.cpsc.gov/",
	"https://www.saferproducts.gov/RestWebServices/Recall?RecallID=1",
	"https://www.cpsc.gov/cgibin/NEISSQuery/home.aspx",
	"https://business.cpsc.gov/robot/",
	"https://www.cpsc.gov/cgibin/labregentry",
	"https://www.cpsc.gov/cgibin/labregentry/labreginfo.aspx",
	"https://apps.saferproducts.gov",
	"https://apps.saferproducts.gov/sspr/public/forgottenPassword",
}

//State is no object per say in go but types are as such
//State type will represent the last knows state of a URL.
type State struct {
	url    string
	status string
}

//Statemonitor maintains a map that stores the state of the URLS being polled
// and prints the current state every updateInterval nanoseconds.
//It returns a chan State to which resource state should be sent.
func StateMonitor(updateInterval time.Duration, smtpconfig config.SMTPConfig) chan<- State {
	updates := make(chan State) //go routines
	urlStatus := make(map[string]string)
	ticker := time.NewTicker(updateInterval)
	go func() {
		for {
			select {
			case <-ticker.C:
				logState(urlStatus, smtpconfig)

			case s := <-updates:
				urlStatus[s.url] = s.status

			}
		}
	}()
	return updates
}

func logState(s map[string]string, smtpConf config.SMTPConfig) {
	if len(s) > 0 {
		log.Println("Current state:")
		for k, v := range s {
			if v != "200 OK" {
				log.Printf(color.RedString("RED ALERT! RED ALERT! RED ALERT! %s %s"), k, v)
			} else {
				log.Printf(color.GreenString("ALL GOOD - %s %s"), k, v)
				delete(s, k)
			}
		}

		if len(s) > 0 {
			sendGridNotification(s, smtpConf)
		}
	} else {
		log.Println("State map object is currently emtpy")
	}
}

//Resouse type represent an HTTP URL to be polled by the program
//this type will report on the uri string passed to it and the error count
type Resource struct {
	url      string
	errCount int
}

//Poller executes an HTTP head request for url and returns the HTTP status string or an error string.
func (r *Resource) Poll() string {
	resp, err := http.Get(r.url)
	if err != nil {
		log.Println("Error", r.url, err)
		r.errCount++
		return err.Error()
	}
	r.errCount = 0
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("error connecting to site")

	}
	strBody := fmt.Sprintf("%s", body)
	if strings.Contains(strBody, strings.ToLower("under maintenance")) {
		resp.Status = "((503 Unavailable))"
	}

	return resp.Status
}

//Sleep sleeps for an appropirate interval (dependent or on error state)
//before sending the resource to done
func (r *Resource) Sleep(done chan<- *Resource) {
	time.Sleep(pollIntervall + errTimeout*time.Duration(r.errCount))
	done <- r
}

//Poller
func Poller(in <-chan *Resource, out chan<- *Resource, status chan<- State) {
	for r := range in {
		s := r.Poll()
		status <- State{r.url, s}
		out <- r
	}

}

func sendNotification(e map[string]string, smtpInfo config.SMTPConfig) {
	// Set up authentication information.

	auth := smtp.PlainAuth("", smtpInfo.Username, smtpInfo.Password, smtpInfo.Hostname)
	var buffer bytes.Buffer
	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	for k, v := range e {
		buffer.WriteString(k + " " + v)
	}

	msg := []byte("To: whom it may concern\r\n" +
		"Subject: WebSite Status!\r\n" +
		"\r\n" +
		buffer.String() + ".\r\n")

	err := smtp.SendMail(smtpInfo.Hostname+smtpInfo.Port, auth, smtpInfo.From, smtpInfo.To, msg)
	if err != nil {
		log.Fatal(err)
	} else {
		log.Println("status of email sent:", err)
	}
}

func sendGridNotification(e map[string]string, smtpInfo config.SMTPConfig) {
	var buffer bytes.Buffer

	for k, v := range e {
		buffer.WriteString(k + " " + v)
	}

	from := mail.NewEmail("Alex Salomon", smtpInfo.From)
	to := mail.NewEmail("Alexandre Salomon", smtpInfo.To[0])
	body := mail.NewContent("text/html", buffer.String())

	subject := "Subject: Cloud WebSite Status alert testing!"
	message := mail.NewV3MailInit(from, subject, to, body)
	request := sendgrid.GetRequest(os.Getenv("SENDGRID_API_KEY"), "/v3/mail/send", "https://api.sendgrid.com")
	request.Method = "POST"
	request.Body = mail.GetRequestBody(message)
	response, err := sendgrid.API(request)
	if err != nil {
		fmt.Println("There was an error")
		fmt.Println(response.StatusCode)
		log.Println(err)
	} else {
		fmt.Println(response.StatusCode)
		fmt.Println(response.Body)
		fmt.Println(response.Headers)
	}

}

func main() {

	var conf config.SMTPConfig

	viper.SetConfigName("config.dev")
	viper.AddConfigPath("config")
	err := viper.ReadInConfig()
	if err != nil {
		log.Println("Config file not found..." + err.Error())
	} else {
		conf = config.SMTPConfig{
			viper.GetString("smtpInfo.hostname"),
			viper.GetString("smtpInfo.password"),
			viper.GetString("smtpInfo.username"),
			viper.GetString("smtpInfo.port"),
			viper.GetString("smtpInfo.from"),
			viper.GetStringSlice("smtpInfo.to"),
		}

	}

	// create input and output channels
	pending, complete := make(chan *Resource), make(chan *Resource)

	//lLaunch the StateMonitor
	status := StateMonitor(statusIntervall, conf)

	//Launch some poller goRoutines
	for i := 0; i < numPollers; i++ {
		go Poller(pending, complete, status)

	}

	//Send some Resources to the pending queue.
	go func() {
		for _, url := range urlsToPoll {
			pending <- &Resource{url: url}
		}

	}()

	for r := range complete {
		go r.Sleep(pending)
	}

}
