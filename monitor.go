package main

import (
        "log"
        "net/http"
        "time"
        "net/smtp"
        "bytes"
        "io/ioutil"
)
const ( 
    //each go routine will be have a task to poll on specified intervals
    numPollers = 3
    pollIntervall = 60 * time.Second
    statusIntervall = 10 * time.Second
    errTimeout = 10 * time.Second
)

var urlsToPoll = []string{
    "http://www.saferproducts.gov/",
    "http://www.cpsc.gov/",
    "https://www.saferproducts.gov/CPSRMSPublic/Industry/Home.aspx/",
    "https://www.saferproducts.gov/CPSRMSPublic/Section15/",
    "https://www.saferproducts.gov/CPSRMSPublic/Incidents/ReportIncident.aspx/",
}

//no objects per say in go but types are as such
//State type will represent the last knows state of a URL.
type State struct{
    url string
    status string
    
}

//Statemonitor maintains a map that stores the state of the URLS being polled
// and prints the current state every updateInterval nanoseconds.
//It returns a chan State to which resource state should be sent.
func  StateMonitor(updateInterval time.Duration) chan <- State {
    updates:= make(chan State) //go routines
    urlStatus := make (map[string]string)
    ticker:= time.NewTicker(updateInterval)
    go func() {
        for {
            select {
                case <-ticker.C:
                      logState(urlStatus)
                      sendNotification(urlStatus)
                case s := <-updates:
                       urlStatus[s.url] = s.status
            
            }
        }
    }()
    return updates
}
    
func logState (s map[string]string){
    log.Println("Current state:")
    for k, v:= range s{
        log.Printf("%s %s", k, v)
    }
}    

func sendNotification(e map[string]string){
    // Set up authentication information.
	hostname := ""
	password := ""
	username := ""
    port := ":25"
    from :=""
	
	auth := smtp.PlainAuth("", username, password, hostname)
    var buffer bytes.Buffer
	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	for k, v:= range e{
        buffer.WriteString( k+ " " + v)
    }
       
	to := []string{""}
	msg := []byte("To: whom it may concern\r\n" +
		"Subject: WebSite Status!\r\n" +
		"\r\n" +
		buffer.String() +".\r\n")
	err := smtp.SendMail(hostname+port , auth, from, to, msg)
	if err != nil {
		log.Fatal(err)
	}
    
    
      
}

//Resouse type represent an HTTP URL to be polled by the program
//this type will report on the uri string passed to it and the error count
type Resource struct{
    url   string
    errCount int
}


//Poller executes an HTTP head request for url and returns the HTTP status string or an error string.
func (r *Resource) Poll() string{
    resp, err := http.Head(r.url)
    if err != nil {
        log.Println("Error", r.url, err)
        r.errCount++
        return err.Error()
    }
    r.errCount = 0
    return resp.Status;
}

//Sleep sleeps for an appropirate interval (dependent or on error state)
//before sending the resource to done
func (r *Resource) Sleep(done chan <- *Resource) {
    time.Sleep(pollIntervall + errTimeout*time.Duration(r.errCount))
    done <- r
}
//Poller
func Poller (in <-chan *Resource, out chan<- *Resource, status chan <- State){
    for r := range in {
        s:= r.Poll()
        status <- State{r.url, s}
        out <- r
    }

}



func main() {
	// create input and output channels
    pending, complete := make (chan * Resource), make(chan * Resource)
    
    //lLaunch the StateMonitor
    status :=StateMonitor(statusIntervall)
    
    //Launch some poller goRoutines
    for i := 0 ;i < numPollers; i ++{
        go Poller(pending, complete, status)
    }
    
    //Send some Resources to the pending queue.
    go func(){
        for _, url :=range urlsToPoll {
            pending <- &Resource{url: url}
        }
        
    }()
    
    for r := range complete {
        go r.Sleep(pending)
    }
}