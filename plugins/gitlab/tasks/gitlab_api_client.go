package tasks

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/merico-dev/lake/config"
	"github.com/merico-dev/lake/logger"
	"github.com/merico-dev/lake/plugins/core"
	"github.com/merico-dev/lake/utils"
)

type GitlabApiClient struct {
	core.ApiClient
}

var gitlabApiClient *GitlabApiClient

func CreateApiClient() *GitlabApiClient {
	if gitlabApiClient == nil {
		gitlabApiClient = &GitlabApiClient{}
		gitlabApiClient.Setup(
			config.V.GetString("GITLAB_ENDPOINT"),
			map[string]string{
				"Authorization": fmt.Sprintf("Bearer %v", config.V.GetString("GITLAB_AUTH")),
			},
			10*time.Second,
			3,
		)
	}
	return gitlabApiClient
}

type GitlabPaginationHandler func(res *http.Response) error

func getTotal(resourceUriFormat string) (int, int, error) {
	// jsut get the first page of results. The response has a head that tells the total pages
	page := 0
	page_size := 1
	res, err := gitlabApiClient.Get(fmt.Sprintf(resourceUriFormat, page_size, page), nil, nil)

	if err != nil {
		resBody, err := ioutil.ReadAll(res.Body)
		if err != nil {
			logger.Error("UnmarshalResponse failed: ", string(resBody))
			return 0, 0, err
		}
		logger.Print(string(resBody) + "\n")
		return 0, 0, err
	}

	totalInt := -1
	total := res.Header.Get("X-Total")
	if total != "" {
		totalInt, err = convertStringToInt(total)
		if err != nil {
			return 0, 0, err
		}
	}

	rateRemaining := res.Header.Get("ratelimit-remaining")
	date, err := http.ParseTime(res.Header.Get("date"))
	if err != nil {
		return 0, 0, err
	}
	rateLimitResetTime, err := http.ParseTime(res.Header.Get("ratelimit-resettime"))
	if err != nil {
		return 0, 0, err
	}
	rateLimitInt, err := strconv.Atoi(rateRemaining)
	if err != nil {
		return 0, 0, err
	}
	rateLimitPerSecond := rateLimitInt / int(rateLimitResetTime.Unix()-date.Unix()) * 9 / 10
	return totalInt, rateLimitPerSecond, nil
}

func convertStringToInt(input string) (int, error) {
	return strconv.Atoi(input)
}

// run all requests in an Ants worker pool
func (gitlabApiClient *GitlabApiClient) FetchWithPaginationAnts(scheduler *utils.WorkerScheduler, resourceUri string, pageSize int, handler GitlabPaginationHandler) error {

	var resourceUriFormat string
	if strings.ContainsAny(resourceUri, "?") {
		resourceUriFormat = resourceUri + "&per_page=%v&page=%v"
	} else {
		resourceUriFormat = resourceUri + "?per_page=%v&page=%v"
	}
	// We need to get the total pages first so we can loop through all requests concurrently
	total, _, err := getTotal(resourceUriFormat)
	if err != nil {
		return err
	}

	// not all api return x-total header, use step concurrency
	if total == -1 {
		// TODO: How do we know how high we can set the conc? Is is rateLimit?
		conc := 10
		step := 0
		c := make(chan bool)
		for {
			for i := conc; i > 0; i-- {
				page := step*conc + i
				err := scheduler.Submit(func() error {
					url := fmt.Sprintf(resourceUriFormat, pageSize, page)
					res, err := gitlabApiClient.Get(url, nil, nil)
					if err != nil {
						return err
					}
					handlerErr := handler(res)
					if handlerErr != nil {
						return handlerErr
					}
					_, err = strconv.ParseInt(res.Header.Get("X-Next-Page"), 10, 32)
					// only send message to channel if I'm the last page
					if page%conc == 0 {
						if err != nil {
							fmt.Println(page, "has no next page")
							c <- false
						} else {
							fmt.Printf("page: %v send true\n", page)
							c <- true
						}
					}
					return nil
				})
				if err != nil {
					return err
				}
			}
			cont := <-c
			if !cont {
				break
			}
			step += 1
		}
	} else {
		// Loop until all pages are requested
		for i := 1; (i * pageSize) <= (total + pageSize); i++ {
			// we need to save the value for the request so it is not overwritten
			currentPage := i
			err1 := scheduler.Submit(func() error {
				url := fmt.Sprintf(resourceUriFormat, pageSize, currentPage)

				res, err := gitlabApiClient.Get(url, nil, nil)

				if err != nil {
					return err
				}

				handlerErr := handler(res)
				if handlerErr != nil {
					return handlerErr
				}
				return nil
			})

			if err1 != nil {
				return err
			}
		}
	}

	scheduler.WaitUntilFinish()
	return nil
}

// fetch paginated without ANTS worker pool
func (gitlabApiClient *GitlabApiClient) FetchWithPagination(resourceUri string, pageSize int, handler GitlabPaginationHandler) error {

	var resourceUriFormat string
	if strings.ContainsAny(resourceUri, "?") {
		resourceUriFormat = resourceUri + "&per_page=%v&page=%v"
	} else {
		resourceUriFormat = resourceUri + "?per_page=%v&page=%v"
	}

	// We need to get the total pages first so we can loop through all requests concurrently
	total, _, _ := getTotal(resourceUriFormat)

	// Loop until all pages are requested
	for i := 0; (i * pageSize) < total; i++ {
		// we need to save the value for the request so it is not overwritten
		currentPage := i
		url := fmt.Sprintf(resourceUriFormat, pageSize, currentPage)

		res, err := gitlabApiClient.Get(url, nil, nil)

		if err != nil {
			return err
		}

		handlerErr := handler(res)
		if handlerErr != nil {
			return handlerErr
		}
	}

	return nil
}
