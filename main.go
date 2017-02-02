package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"

	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/net/html"
)

var settings struct {
	uRL, username, password string
}

const redirect = "&redirect=1"
const stdin = 1

func init() {
	flag.StringVar(&settings.uRL, "u", "", "the url of the moodle page")
	flag.StringVar(&settings.username, "n", "", "your moodle username")
	flag.StringVar(&settings.password, "p", "", "your moodle password")

	flag.Parse()
}

func main() {
	if settings.uRL == "" || settings.username == "" {
		flag.Usage()
		os.Exit(1)
	}

	password, err := getPassword()
	if err != nil {
		log.Fatal(err)
	}

	resource, err := url.Parse(settings.uRL)
	if err != nil {
		log.Fatal(err)
	}

	client, err := authenticatedClient(resource.Host, settings.username, password)
	if err != nil {
		log.Fatal(err)
	}

	links, err := getRsourceLinks(client)
	if err != nil {
		log.Fatal(err)
	}

	done := make(chan struct{}, 0)
	for _, url := range links {
		go downloadFiles(url, client, done)
	}
	for range links {
		<-done
	}
}

func downloadFiles(url string, client *http.Client, done chan<- struct{}) {
	resp, err := client.Get(url + redirect)
	if err != nil {
		log.Println(err)
		done <- struct{}{}
	}
	defer resp.Body.Close()
	tokens := strings.Split(resp.Request.URL.String(), "/")
	filename := tokens[len(tokens)-1]

	text, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
		done <- struct{}{}
	}
	err = ioutil.WriteFile(filename, text, 0666)
	if err != nil {
		log.Println(err)
	}
	done <- struct{}{}
}

func getRsourceLinks(client *http.Client) ([]string, error) {
	resp, err := client.Get(settings.uRL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	text, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err = checkLoginSuccessful(text); err != nil {
		return nil, err
	}

	tokens := html.NewTokenizer(bytes.NewReader(text))

	links := make([]string, 0, 100)
	for tokens.Next() != html.ErrorToken {
		token := tokens.Token()
		if token.Data == "a" {
			if strings.Contains(token.String(), "mod/resource") {
				for _, attr := range token.Attr {
					if attr.Key == "href" {
						links = append(links, attr.Val)
					}
				}
			}
		}
	}
	return links, nil
}

func checkLoginSuccessful(body []byte) error {

	if strings.Contains(string(body), "Gäste dürfen nicht auf diesen Kurs zugreifen. Melden Sie sich bitte an.") {
		return fmt.Errorf("Login failed")
	}
	return nil
}

func authenticatedClient(mURL, username, password string) (*http.Client, error) {
	var err error
	client := &http.Client{}
	client.Jar, err = cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.PostForm("https://"+mURL+"/login/index.php", url.Values{
		"username": {username},
		"password": {password},
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return client, nil
}

func getPassword() (string, error) {
	if settings.password != "" {
		return settings.password, nil
	}
	secret, err := getSecretFromTerminal()
	return string(secret), err
}

func getSecretFromTerminal() ([]byte, error) {

	fmt.Print("Enter your moodle password: ")
	return getOneFromTerminalSecret()
}

func getOneFromTerminalSecret() ([]byte, error) {

	oldState, err := terminal.MakeRaw(stdin)
	if err != nil {
		return nil, err
	}
	defer terminal.Restore(stdin, oldState)

	return terminal.ReadPassword(stdin)
}
