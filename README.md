# ghprs

Simple github terminal tool for listing an organization's open pull requests.

### Usage

Build and install in GOPATH/bin:

    go get -v github.com/StefanKjartansson/ghprs

Create a `.ghprs` configuration file in your home dir containing the name of your organization & an oauth token from github.

    organization = "myorg"
    token = "myoauthtokenfromgithub"

Run:

    ghprs

