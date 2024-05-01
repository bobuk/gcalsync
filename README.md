# 📆 gcalsync: Sync Your Google Calendars Like a Boss! 🚀

[![Go Version](https://img.shields.io/badge/go-1.16+-00ADD8?style=flat-square&logo=go)](https://golang.org/)
[![License](https://img.shields.io/badge/license-MIT-0969da?style=flat-square&logo=opensource)](https://opensource.org/licenses/MIT)

Welcome to **gcalsync**, the ultimate tool for syncing your Google Calendars across multiple accounts!
Say goodbye to calendar conflicts and hello to seamless synchronization. 🎉

## 🌟 Features

-   🔄 Sync events from multiple Google Calendars across different accounts
-   🚫 Create "blocker" events in other calendars to prevent double bookings
-   🗄️ Store access tokens and calendar data securely in a local SQLite database
-   🔒 Authenticate with Google using the OAuth2 flow for desktop apps
-   🧹 Easy way to cleanup calendars and remove all blocker events with a single command

## 📋 Prerequisites

-   Go 1.16 or higher
-   A Google Cloud Platform project with the Google Calendar API enabled
-   OAuth2 credentials (client ID and client secret) for the desktop app flow

## 🚀 Getting Started

1. Clone the repository:

    ```
    git clone https://github.com/bobuk/gcalsync.git
    ```

2. Navigate to the project directory:

    ```
    cd gcalsync
    ```

3. Install the dependencies:

    ```
    go mod download
    ```

4. Create a `.gcalsync.toml` file in the project directory with your OAuth2 credentials:

    ```toml
    client_id = "your-client-id"
    client_secret = "your-client-secret"
    ```

    Don't forget to choose the appropriate OAuth2 consent screen settings and add the necessary scopes for the Google Calendar API, also double check that you are select "Desktop app" as application type.

    You can move the file to `~/.config/gcalsync/.gcalsync.toml` to avoid storing sensitive data in the project directory. In this case your database file will be created in `~/.config/gcalsync/` as well.

5. Build the executable:

    ```
    go build
    ```

6. Run the `gcalsync` command with the desired action:
    - To add a new calendar:
        ```
        ./gcalsync add
        ```
    - To sync calendars:
        ```
        ./gcalsync sync
        ```
    - To desync calendars:
        ```
        ./gcalsync desync
        ```
    - To list all calendars:
        ```
        ./gcalsync list
        ```

## 📚 Documentation

### 🆕 Adding a Calendar

To add a new calendar to sync, run the `gcalsync add` command. You will be prompted to enter the account name and calendar ID. The program will guide you through the OAuth2 authentication process and store the access token securely in the local database.

### 🔄 Syncing Calendars

To sync your calendars, run the `gcalsync sync` command. The program will retrieve events from the specified calendars within the current and next month time window. It will create "blocker" events in other calendars to prevent double bookings and store the blocker event details in the local database.

### 🧹 Desyncing Calendars

To desync your calendars and remove all blocker events, run the `gcalsync desync` command. The program will retrieve the blocker event details from the local database and remove the corresponding events from the respective calendars.

### 📋 Listing Calendars

To list all calendars that have been added to the local database, run the `gcalsync list` command. The program will display the account name and calendar ID for each calendar.

### 🎗️ Disabling Reminders

By default blocker events will inherit your default Google Calendar reminder/alert settings (typically – 10 minutes before the event). If you *do not want* to receive reminders for the blocker events, you can disable them by setting the `disable_reminders` field to `true` in the `.gcalsync.toml` configuration file.

## 🤝 Contributing

Contributions are welcome! If you encounter any issues or have suggestions for improvement, please open an issue or submit a pull request. Let's make gcalsync even better together! 💪

## 📄 License

This project is licensed under the [MIT License](https://opensource.org/licenses/MIT). Feel free to use, modify, and distribute the code as you see fit. We hope you find it useful! 🌟

## 🙏 Acknowledgements

-   The terrible [Go](https://golang.org/) programming language
-   The [Google Calendar API](https://developers.google.com/calendar) for making this project almost impossible to implement
-   The [OAuth2](https://oauth.net/2/) protocol for very missleading but secure authentication
-   The [SQLite](https://www.sqlite.org/) database for lightweight and efficient storage, the only one that added no pain.
