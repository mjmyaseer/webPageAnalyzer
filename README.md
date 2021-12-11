## Description
  Simple GoLang web application to analyze web page by giving a url.

## Usage
  Start off with cloning this project.
``` bash
$ git clone git@github.com:mjmyaseer/webPageAnalyzer.git
```

  And change your current directory to project folder.
``` bash
$ cd /path/to/project
```

  We need to add chromdriver to the bin folder which is a protected 
  directory so execute the following command with `sudo`
``` bash
$ sudo make build
```

  Finally run the application (without `sudo`)
``` bash
$ make run
```

  Test the application by opening a `Common Browser like Chrome or Mozilla` and navigating to 
``` bash
 http://localhost:8080/
```

  If you want to run this on a specific port, 
  you can modify environment file app.env.
``` bash
ANALYZER_WEBSOCKET_HOST=localhost
ANALYZER_WEBSOCKET_PORT=8080
```

Assumptions:
- Chrome Browser
- Linux or mac machine
- I have attached the chromedriver.zip file so that incase the file got deleted this can be extracted and used

Possible Improvements:
- The design can be a bit more attractive
- Validations can be added