package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"net/url"
	"os"

	"github.com/gorilla/sessions"
	"github.com/joho/godotenv"
	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
	"github.com/markbates/goth/providers/facebook"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("⚠️ .env not loaded, using system env")
	}

	key := os.Getenv("SESSION_SECRET")
	if key == "" {
		log.Fatal("❌ SESSION_SECRET is missing in .env")
	}
	log.Println("✅ SESSION_SECRET loaded successfully!")

	store := sessions.NewCookieStore([]byte(key))
	store.MaxAge(86400 * 30)
	store.Options.HttpOnly = true
	store.Options.Secure = false
	gothic.Store = store

	goth.UseProviders(
		facebook.New(
			os.Getenv("679702924653299"),
			os.Getenv("3a9c276438e3e23a511430d143bb263d"),
			"http://localhost:8080/auth/facebook/callback",
		),
		google.New(
			os.Getenv("94664221445-366tf0i5en6p3n9ho0185q3gq8s54q03.apps.googleusercontent.com"),
			os.Getenv("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"),
			"http://localhost:8080/auth/google/callback",
			"email", "profile",
		),
		github.New(
			os.Getenv("Ov23liZvKea7vJ6Zhxj9"),
			os.Getenv("53ecfb47a14a5f4f639bf74f6990f6d20ee3ef24"),
			"http://localhost:8080/auth/github/callback",
		),
	)

	db, err = sql.Open("sqlite", "./subscribers/DB_subscribers.db")
	if err != nil {
		log.Fatal("❌ DB connection failed:", err)
	}
	defer db.Close()
	createTables()

	// http.Handle("/", http.FileServer(http.Dir("./static")))
	http.HandleFunc("/", serveIndex)
	http.HandleFunc("/subscribe", serveSubscribe)
	http.HandleFunc("/subscribe/email", handleEmailSubscription)
	http.HandleFunc("/subscribers", handleListSubscribers)
	http.HandleFunc("/view-emails", handleViewEmails)
	http.HandleFunc("/submit", handleFormSubmission)

	// http.HandleFunc("/auth/facebook", handleFacebookLogin("facebook"))
	// http.HandleFunc("/auth/facebook/callback", handleFacebookCallback("facebook"))
	// http.HandleFunc("/auth/google", handleGoogleLogin("google"))
	// http.HandleFunc("/auth/google/callback", handleGoogleCallback("google"))
	// http.HandleFunc("/auth/github", handleGitHubLogin("github"))
	// http.HandleFunc("/auth/github/callback", handleGitHubCallback("github"))

	http.HandleFunc("/auth/facebook", handleFacebookLogin)
	http.HandleFunc("/auth/facebook/callback", handleFacebookCallback)

	http.HandleFunc("/auth/google", handleGoogleLogin)
	http.HandleFunc("/auth/google/callback", handleGoogleCallback)

	http.HandleFunc("/auth/github", handleGitHubLogin)
	http.HandleFunc("/auth/github/callback", handleGitHubCallback)

	log.Println("🌐 Server started at http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func createTables() {
	subscriberTable := `CREATE TABLE IF NOT EXISTS subscribers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		email TEXT UNIQUE NOT NULL
	);`
	messageTable := `CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subscriber_id INTEGER,
		message TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(subscriber_id) REFERENCES subscribers(id)
	);`

	_, err := db.Exec(subscriberTable)
	if err != nil {
		log.Fatal("❌ Failed to create subscribers table:", err)
	}
	_, err = db.Exec(messageTable)
	if err != nil {
		log.Fatal("❌ Failed to create messages table:", err)
	}
}


func serveIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
		return
	}
	http.ServeFile(w, r, "index.html")
}


func serveSubscribe(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		http.ServeFile(w, r, "./static/subscribe.html")
	} else {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
	}
}

func handleEmailSubscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
		return
	}
	email := r.FormValue("email")
	message := r.FormValue("message")

	if email == "" || message == "" {
		http.Error(w, "Email and message are required", http.StatusBadRequest)
		return
	}

	// Step 1: Insert or ignore subscriber
	_, err := db.Exec("INSERT OR IGNORE INTO subscribers(email) VALUES(?)", email)
	if err != nil {
		http.Error(w, "❌ Could not save email: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 2: Get subscriber ID
	var id int
	err = db.QueryRow("SELECT id FROM subscribers WHERE email = ?", email).Scan(&id)
	if err != nil {
		http.Error(w, "❌ Could not fetch ID: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 3: Insert message
	_, err = db.Exec("INSERT INTO messages(subscriber_id, message) VALUES(?, ?)", id, message)
	if err != nil {
		http.Error(w, "❌ Could not save message: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Step 4: Save email to .txt file
	f, err := os.OpenFile("subscribers_emails.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(email + "\n")
	}

	// Step 5: Send confirmation email
	link := "http://localhost:8080/verify?email=" + url.QueryEscape(email)
	sendConfirmationEmail(email, link)

	fmt.Fprintf(w, "✅ Thanks %s! Confirmation sent.", email)
}

func sendConfirmationEmail(to string, link string) {
	from := os.Getenv("victor.via7@gmail.com")
	password := os.Getenv("juvi wwyx rdkh qdjc")

	subject := "Please verify your email"
	body := fmt.Sprintf("Click the link to confirm:\n%s", link)

	msg := "From: " + from + "\nTo: " + to + "\nSubject: " + subject + "\n\n" + body

	err := smtp.SendMail("smtp.gmail.com:587",
		smtp.PlainAuth("", from, password, "smtp.gmail.com"),
		from, []string{to}, []byte(msg))

	if err != nil {
		log.Println("❌ Email send failed:", err)
	} else {
		log.Println("✅ Email sent to:", to)
	}
}

func handleListSubscribers(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT email FROM subscribers")
	if err != nil {
		http.Error(w, "Failed to fetch subscribers", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var email string
		rows.Scan(&email)
		fmt.Fprintln(w, email)
	}
}

func handleViewEmails(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile("subscribers_emails.txt")
	if err != nil {
		http.Error(w, "❌ Cannot read file", http.StatusInternalServerError)
		return
	}
	w.Write(data)
}

func handleFormSubmission(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		r.ParseForm()
		email := r.FormValue("email")
		message := r.FormValue("message")
		fmt.Printf("📩 New message from %s: %s\n", email, message)
		w.Write([]byte("✅ Message received!"))
	} else {
		http.Error(w, "Invalid method", http.StatusMethodNotAllowed)
	}
}

// OAuth handlers

func handleFacebookLogin(w http.ResponseWriter, r *http.Request) {
	r.URL.RawQuery = "provider=facebook"
	gothic.BeginAuthHandler(w, r)
}

func handleFacebookCallback(w http.ResponseWriter, r *http.Request) {
	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		http.Error(w, "Facebook Login failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "✅ Facebook Login Successful!\nName: %s\nEmail: %s\n", user.Name, user.Email)
}

func handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	r.URL.RawQuery = "provider=google"
	gothic.BeginAuthHandler(w, r)
}

func handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		http.Error(w, "Google Login failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "✅ Google Login Successful!\n\nName: %s\nEmail: %s\n", user.Name, user.Email)
}

func handleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	r.URL.RawQuery = "provider=github"
	gothic.BeginAuthHandler(w, r)
}

func handleGitHubCallback(w http.ResponseWriter, r *http.Request) {
	user, err := gothic.CompleteUserAuth(w, r)
	if err != nil {
		http.Error(w, "GitHub Login failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(w, "✅ GitHub Login Successful!\nName: %s\nEmail: %s\n", user.Name, user.Email)
}

// func handleFacebookLogin(provider string) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		r = r.WithContext(context.WithValue(r.Context(), gothic.ProviderParamKey, provider))
// 		gothic.BeginAuthHandler(w, r)
// 	}
// }

// func handleFacebookCallback(provider string) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		r = r.WithContext(context.WithValue(r.Context(), gothic.ProviderParamKey, provider))
// 		user, err := gothic.CompleteUserAuth(w, r)
// 		if err != nil {
// 			http.Error(w, provider+" login failed: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		fmt.Fprintf(w, "✅ Logged in via %s\nName: %s\nEmail: %s", provider, user.Name, user.Email)
// 	}
// }

// func handleGoogleLogin(provider string) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		r = r.WithContext(context.WithValue(r.Context(), gothic.ProviderParamKey, provider))
// 		gothic.BeginAuthHandler(w, r)
// 	}
// }

// func handleGoogleCallback(provider string) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		r = r.WithContext(context.WithValue(r.Context(), gothic.ProviderParamKey, provider))
// 		user, err := gothic.CompleteUserAuth(w, r)
// 		if err != nil {
// 			http.Error(w, provider+" login failed: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		fmt.Fprintf(w, "✅ Logged in via %s\nName: %s\nEmail: %s", provider, user.Name, user.Email)
// 	}
// }

// func handleGitHubLogin(provider string) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		r = r.WithContext(context.WithValue(r.Context(), gothic.ProviderParamKey, provider))
// 		gothic.BeginAuthHandler(w, r)
// 	}
// }

// func handleGitGubCallback(provider string) http.HandlerFunc {
// 	return func(w http.ResponseWriter, r *http.Request) {
// 		r = r.WithContext(context.WithValue(r.Context(), gothic.ProviderParamKey, provider))
// 		user, err := gothic.CompleteUserAuth(w, r)
// 		if err != nil {
// 			http.Error(w, provider+" login failed: "+err.Error(), http.StatusInternalServerError)
// 			return
// 		}
// 		fmt.Fprintf(w, "✅ Logged in via %s\nName: %s\nEmail: %s", provider, user.Name, user.Email)
// 	}
// }
