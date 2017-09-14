# slack-dump
Generate an export of Channel, Private Group and / or Direct Message history and export it as a ZIP file compatible with Slack's import tool.

## Usage

```
$ slack-dump -h

NAME:
   slack-dump - export channel, group and direct message history to the Slack export format

USAGE:
   slack-dump [global options] command [command options] [arguments...]

VERSION:
   0.0.2

AUTHOR(S):
   Joe Fitzgerald <jfitzgerald@pivotal.io>
   Sunyong Lim <dicebattle@gmail.com>

COMMANDS:
   help, h	Shows a list of commands or help for one command

GLOBAL OPTIONS:
   --token, -t 		a Slack API token: (see: https://api.slack.com/web) [$SLACK_API_TOKEN]
   --help, -h		show help
   --version, -v	print the version
   --text, -x		do the plain text dump too
```

### Export All Channels And Private Groups

```
$ slack-dump -t=YOURSLACKAPITOKENISHERE
```

### Export Specific Channels And Private Groups

```
$ slack-dump -t=YOURSLACKAPITOKENISHERE channel-name-here privategroup-name-here another-privategroup-name-here
```
