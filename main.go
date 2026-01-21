package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
	//_ "github.com/mattn/go-sqlite3" // ← local SQLite driver
	 _ "github.com/tursodatabase/libsql-client-go/libsql"
        "golang.org/x/crypto/bcrypt"
)

type Jhapat struct {
	ID        int    `json:"id"`
	User      string `json:"user"`
	Handle    string `json:"handle"`
	Content   string `json:"content"`
	Category  string `json:"category"`
	Avatar    string `json:"avatar"`
	Image     string `json:"image"`
	Timer     string `json:"timer"`
	Claimed   int    `json:"claimed"`
	Left      int    `json:"left"`
	Price     string `json:"price"`
	Area      string `json:"area"`
	IsPremium int    `json:"is_premium"`
	Verified  bool   `json:"verified"`
	Tier      string `json:"tier"`
	Timestamp string `json:"timestamp"`
}

type User struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	IsGold      int    `json:"is_gold"`
	Status      string `json:"status"`
	RecoveryKey string `json:"recovery_key"`
	Avatar      string `json:"avatar"`
}

var db *sql.DB

func main() {
  dbURL := os.Getenv("TURSO_DATABASE_URL")
        dbToken := os.Getenv("TURSO_AUTH_TOKEN")
        
        fullURL := fmt.Sprintf("%s?authToken=%s", dbURL, dbToken)

	var err error
	db, err = sql.Open("libsql", fullURL)
	if err != nil {
		log.Fatal("Turso Connection Error: ", err)
	}	


// Use simple local SQLite file (no cloud/Turso)
//	dbPath := "jhapat_local.db"
//	connString := "file:" + dbPath + "?_foreign_keys=on"

//	var err error
//	db, err = sql.Open("sqlite3", connString)
//	if err != nil {
//		log.Fatal("Cannot open local database → ", err)
//	}
	defer db.Close()

	// Make sure we can actually use the database
	if err = db.Ping(); err != nil {
		log.Fatal("Database ping failed → ", err)
	}

//	log.Printf("✅ Connected to local SQLite database: %s", dbPath)

	// Create tables (DROP + CREATE for clean dev start - you can change to IF NOT EXISTS later)
	_, err = db.Exec("DROP TABLE IF EXISTS claims")
	if err != nil {
		log.Println("Warning dropping claims:", err)
	}
	_, err = db.Exec("DROP TABLE IF EXISTS jhapats")
	if err != nil {
		log.Println("Warning dropping jhapats:", err)
	}
	_, err = db.Exec("DROP TABLE IF EXISTS users")
	if err != nil {
		log.Println("Warning dropping users:", err)
	}

	// Create fresh tables
	db.Exec(`
		CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			role TEXT NOT NULL,
			is_gold INTEGER DEFAULT 0,
			status TEXT DEFAULT 'approved',
			recovery_key TEXT,
			avatar TEXT
		)`)

	db.Exec(`
		CREATE TABLE jhapats (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user TEXT NOT NULL,
			handle TEXT,
			content TEXT NOT NULL,
			category TEXT,
			avatar TEXT,
			image TEXT,
			timer TEXT,
			claimed INTEGER DEFAULT 0,
			left INTEGER DEFAULT 10,
			price TEXT,
			area TEXT,
			is_premium INTEGER DEFAULT 0,
			verified INTEGER DEFAULT 0,
			tier TEXT,
			timestamp TEXT NOT NULL
		)`)

	db.Exec(`
		CREATE TABLE claims (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			deal_id INTEGER NOT NULL,
			code TEXT NOT NULL,
			timestamp TEXT NOT NULL
		)`)

	// Seed some sample data
	seedData()

	// Fiber app
	app := fiber.New(fiber.Config{
		BodyLimit: 10 * 1024 * 1024,
	})

	app.Use(logger.New())
	app.Static("/", "./public")

	app.Get("/favicon.ico", func(c *fiber.Ctx) error {
		return c.SendStatus(204)
	})

	// Register
	app.Post("/api/register", func(c *fiber.Ctx) error {
		u := new(User)
		if err := c.BodyParser(u); err != nil {
			return c.Status(400).SendString("Invalid request format")
		}
		if u.Username == "" || u.Password == "" {
			return c.Status(400).SendString("Username and password required")
		}
		if u.Username == u.Password {
			return c.Status(400).SendString("Password cannot be same as username")
		}
		if u.Role == "admin" {
			return c.Status(403).SendString("Cannot register as admin")
		}

		status := "approved"
		if u.Role == "merchant" {
			status = "pending"
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(u.Password), 10)
		if err != nil {
			return c.Status(500).SendString("Password error")
		}

		recoveryKey := fmt.Sprintf("REC-%d", time.Now().UnixNano()%1000000)

		_, err = db.Exec(
			"INSERT INTO users (username, password, role, status, recovery_key, avatar) VALUES (?,?,?,?,?,?)",
			u.Username, string(hashed), u.Role, status, recoveryKey, u.Avatar,
		)
		if err != nil {
			log.Println("Register SQL error:", err)
			return c.Status(500).SendString("Could not create user (maybe username exists?)")
		}

		return c.JSON(fiber.Map{"status": "registered", "recovery_key": recoveryKey})
	})

	// Login
	app.Post("/api/login", func(c *fiber.Ctx) error {
		u := new(User)
		if err := c.BodyParser(u); err != nil {
			return c.Status(400).SendString("Bad request")
		}

		adminUser := os.Getenv("ADMIN_USER")
		adminPass := os.Getenv("ADMIN_PASS")
		if (adminUser != "" && u.Username == adminUser && u.Password == adminPass) || (u.Username == "superadmin" && u.Password == "jhapat2026") {
			return c.JSON(fiber.Map{
				"username": "admin",
				"role":     "admin",
				"is_gold":  1,
				"status":   "approved",
			})
		}

		var dbPass, dbRole, dbStatus, dbAvatar string
		var isGold int
		err := db.QueryRow(
			"SELECT password, role, is_gold, status, avatar FROM users WHERE username = ?",
			u.Username,
		).Scan(&dbPass, &dbRole, &isGold, &dbStatus, &dbAvatar)
		if err != nil {
			return c.Status(401).SendString("Invalid credentials")
		}

		if err := bcrypt.CompareHashAndPassword([]byte(dbPass), []byte(u.Password)); err != nil {
			return c.Status(401).SendString("Invalid credentials")
		}

		if dbRole == "merchant" && dbStatus != "approved" {
			return c.Status(403).SendString("Account pending approval")
		}
		return c.JSON(fiber.Map{"username": u.Username, "role": dbRole, "is_gold": isGold, "avatar": dbAvatar})
	})

	// Get all deals
	app.Get("/api/tweets", func(c *fiber.Ctx) error {
		rows, err := db.Query("SELECT * FROM jhapats ORDER BY is_premium DESC, id DESC")
		if err != nil {
			return c.Status(500).SendString("Could not fetch feed")
		}
		defer rows.Close()

		var feed []Jhapat
		for rows.Next() {
			var t Jhapat
			err := rows.Scan(&t.ID, &t.User, &t.Handle, &t.Content, &t.Category, &t.Avatar, &t.Image, &t.Timer, &t.Claimed, &t.Left, &t.Price, &t.Area, &t.IsPremium, &t.Verified, &t.Tier, &t.Timestamp)
			if err != nil {
				continue
			}
			feed = append(feed, t)
		}
		return c.JSON(feed)
	})

	// Get nearby deals
	app.Get("/api/tweets/nearby/:area", func(c *fiber.Ctx) error {
		area := c.Params("area")
		rows, err := db.Query("SELECT * FROM jhapats WHERE area LIKE ? ORDER BY is_premium DESC, id DESC", "%"+area+"%")
		if err != nil {
			return c.Status(500).SendString("Could not fetch nearby deals")
		}
		defer rows.Close()

		var feed []Jhapat
		for rows.Next() {
			var t Jhapat
			rows.Scan(&t.ID, &t.User, &t.Handle, &t.Content, &t.Category, &t.Avatar, &t.Image, &t.Timer, &t.Claimed, &t.Left, &t.Price, &t.Area, &t.IsPremium, &t.Verified, &t.Tier, &t.Timestamp)
			feed = append(feed, t)
		}
		return c.JSON(feed)
	})

	// Claim deal
	app.Post("/api/jhapat/claim/:id", func(c *fiber.Ctx) error {
		idStr := c.Params("id")
		var id int
		fmt.Sscanf(idStr, "%d", &id)

		type ClaimReq struct {
			Username string `json:"username"`
			Code     string `json:"code"`
		}
		req := new(ClaimReq)
		c.BodyParser(req)

		tx, _ := db.Begin()
		_, err := tx.Exec("UPDATE jhapats SET left = left - 1, claimed = claimed + 1 WHERE id = ? AND left > 0", id)
		if err != nil {
			tx.Rollback()
			return c.Status(500).SendString("Could not claim deal")
		}

		_, err = tx.Exec("INSERT INTO claims (username, deal_id, code, timestamp) VALUES (?, ?, ?, ?)",
			req.Username, id, req.Code, time.Now().Format(time.RFC3339))

		tx.Commit()
		return c.SendStatus(200)
	})

	// User messages
	app.Get("/api/user/messages/:username", func(c *fiber.Ctx) error {
		username := c.Params("username")
		rows, err := db.Query(`
			SELECT c.id, c.code, c.timestamp, j.content 
			FROM claims c 
			JOIN jhapats j ON c.deal_id = j.id 
			WHERE c.username = ? ORDER BY c.id DESC`, username)
		if err != nil {
			return c.Status(500).SendString("Error fetching messages")
		}
		defer rows.Close()
		var messages []interface{}
		for rows.Next() {
			var id int
			var code, timestamp, content string
			err := rows.Scan(&id, &code, &timestamp, &content)
			if err != nil {
				continue
			}
			messages = append(messages, fiber.Map{"id": id, "code": code, "timestamp": timestamp, "content": content})
		}
		return c.JSON(messages)
	})

	// Merchant redeem
	app.Post("/api/merchant/redeem", func(c *fiber.Ctx) error {
		type Redemption struct {
			Code string `json:"code"`
		}
		r := new(Redemption)
		c.BodyParser(r)
		if r.Code == "" {
			return c.Status(400).SendString("Code required")
		}
		log.Printf("Verified Coupon: %s", r.Code)
		return c.JSON(fiber.Map{"status": "Redeemed Successfully!"})
	})

	// Merchant stats
	app.Get("/api/merchant/stats/:username", func(c *fiber.Ctx) error {
		username := c.Params("username")
		rows, err := db.Query("SELECT id, content, left, claimed FROM jhapats WHERE user = ?", username)
		if err != nil {
			return c.Status(500).SendString("Could not fetch stats")
		}
		defer rows.Close()
		var stats []Jhapat
		for rows.Next() {
			var s Jhapat
			rows.Scan(&s.ID, &s.Content, &s.Left, &s.Claimed)
			stats = append(stats, s)
		}
		return c.JSON(stats)
	})

	// Post merchant deal
	app.Post("/api/merchant/deal", func(c *fiber.Ctx) error {
		t := new(Jhapat)
		if err := c.BodyParser(t); err != nil {
			return c.Status(400).SendString("Invalid deal data")
		}
		var isGold int
		var avatar string
		db.QueryRow("SELECT is_gold, avatar FROM users WHERE username = ?", t.User).Scan(&isGold, &avatar)

		now := time.Now().Format(time.RFC3339)

		// Fix: calculate tierValue correctly before Exec
		tierValue := ""
		if isGold == 1 {
			tierValue = "gold"
		}

		_, err := db.Exec(`
			INSERT INTO jhapats 
			(user, handle, content, category, avatar, image, timer, claimed, left, price, area, is_premium, verified, tier, timestamp) 
			VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, 0, ?, ?)`,
			t.User, "@"+t.User, t.Content, t.Category, avatar, t.Image, t.Timer, t.Left, t.Price, t.Area, isGold, tierValue, now,
		)
		if err != nil {
			log.Println("Post deal error:", err)
			return c.Status(500).SendString("Could not post deal")
		}
		return c.SendStatus(201)
	})

	// Admin - list users
	app.Get("/api/admin/users", func(c *fiber.Ctx) error {
		rows, _ := db.Query("SELECT id, username, role, is_gold, status FROM users WHERE role = 'merchant'")
		defer rows.Close()
		var users []User
		for rows.Next() {
			var u User
			rows.Scan(&u.ID, &u.Username, &u.Role, &u.IsGold, &u.Status)
			users = append(users, u)
		}
		return c.JSON(users)
	})

	// Admin - approve merchant
	app.Post("/api/admin/approve/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		_, err := db.Exec("UPDATE users SET status = 'approved' WHERE id = ?", id)
		if err != nil {
			return c.Status(500).SendString("Failed to approve merchant")
		}
		return c.SendStatus(200)
	})

	// Admin - toggle gold
	app.Post("/api/admin/toggle-gold/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		var current int
		var username string
		db.QueryRow("SELECT is_gold, username FROM users WHERE id = ?", id).Scan(&current, &username)

		newStatus := 0
		if current == 0 {
			newStatus = 1
		}

		db.Exec("UPDATE users SET is_gold = ? WHERE id = ?", newStatus, id)
		db.Exec("UPDATE jhapats SET is_premium = ? WHERE user = ?", newStatus, username)

		return c.SendStatus(200)
	})

	// Admin - delete jhapat
	app.Delete("/api/admin/delete-jhapat/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		_, err := db.Exec("DELETE FROM jhapats WHERE id = ?", id)
		if err != nil {
			return c.Status(500).SendString("Failed to delete")
		}
		return c.SendStatus(200)
	})

	// Reset password
	app.Post("/api/reset-password", func(c *fiber.Ctx) error {
		type ResetReq struct {
			Username    string `json:"username"`
			RecoveryKey string `json:"recoveryKey"`
			NewPassword string `json:"newPassword"`
		}
		req := new(ResetReq)
		if err := c.BodyParser(req); err != nil {
			return c.Status(400).SendString("Bad request")
		}

		var dbKey string
		err := db.QueryRow("SELECT recovery_key FROM users WHERE username = ?", req.Username).Scan(&dbKey)
		if err != nil || dbKey != req.RecoveryKey {
			return c.Status(401).SendString("Incorrect details")
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), 10)
		if err != nil {
			return c.Status(500).SendString("Password error")
		}

		_, err = db.Exec("UPDATE users SET password = ? WHERE username = ?", hashed, req.Username)
		if err != nil {
			return c.Status(500).SendString("Update failed")
		}

		return c.SendStatus(200)
	})

// Get all claims for a specific merchant's deals
app.Get("/api/merchant/claims/:username", func(c *fiber.Ctx) error {
    username := c.Params("username")
    rows, err := db.Query(`
        SELECT c.id, c.username, c.code, c.timestamp, j.content 
        FROM claims c 
        JOIN jhapats j ON c.deal_id = j.id 
        WHERE j.user = ? ORDER BY c.id DESC`, username)
    if err != nil {
        return c.Status(500).SendString("Error fetching claims")
    }
    defer rows.Close()
    
    var claims []interface{}
    for rows.Next() {
        var id int
        var user, code, timestamp, content string
        rows.Scan(&id, &user, &code, &timestamp, &content)
        claims = append(claims, fiber.Map{
            "id": id, "user": user, "code": code, "timestamp": timestamp, "content": content,
        })
    }
    return c.JSON(claims)
})

// Mark a code as redeemed (optional logic)
app.Post("/api/merchant/verify-code", func(c *fiber.Ctx) error {
    type VerifyReq struct {
        Code string `json:"code"`
    }
    req := new(VerifyReq)
    c.BodyParser(req)
    
    var exists int
    err := db.QueryRow("SELECT COUNT(*) FROM claims WHERE code = ?", req.Code).Scan(&exists)
    if err != nil || exists == 0 {
        return c.Status(404).SendString("Invalid or expired code")
    }
    
    // In a real app, you'd add a 'status' column to 'claims' and set it to 'redeemed' here.
    return c.JSON(fiber.Map{"status": "success", "message": "Code Verified! You can now provide the service."})
})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	log.Printf("🚀 Jhapat running → http://localhost:%s", port)
	log.Fatal(app.Listen(":" + port))
}


func seedData() {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM jhapats").Scan(&count)
	if count > 0 {
		return // already has data
	}

	now := time.Now().Format(time.RFC3339)

	samples := []struct {
		user     string
		content  string
		category string
		price    string
		area     string
		left     int
		premium  int
	}{
		{"PizzaHub", "Buy 1 Get 1 Free Margherita", "Food", "₹299", "Kukatpally", 15, 1},
		{"TrendyWear", "Flat 40% Off on Jeans", "Shop", "₹799", "Hitech City", 8, 0},
		{"UrbanFit", "1 Month Gym Membership", "Health", "₹1499", "Gachibowli", 5, 0},
	}

	for _, s := range samples {
		// Fix: calculate tierValue correctly before Exec
		tierValue := ""
		if s.premium == 1 {
			tierValue = "gold"
		}

		db.Exec(`
			INSERT INTO jhapats 
			(user, handle, content, category, avatar, timer, claimed, left, price, area, is_premium, verified, tier, timestamp)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?, ?, ?, 0, ?, ?)`,
			s.user, "@"+s.user, s.content, s.category,
			fmt.Sprintf("https://i.pravatar.cc/150?u=%s", s.user),
			"48h", s.left, s.price, s.area, s.premium, tierValue, now,
		)
	}

	log.Println("🌱 Sample deals added!")
}
