package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/logger"
        _ "github.com/tursodatabase/libsql-client-go/libsql"
	"golang.org/x/crypto/bcrypt"
)

type Jhapat struct {
	ID        int    `json:"id"`
	User      string `json:"user"`
	Handle    string `json:"handle"`
	Content   string `json:"content"`
	Category  string `json:"category"` // Added dedicated category field
	Avatar    string `json:"avatar"`
	Image     string `json:"image"`
	Timer     string `json:"timer"`
	Claimed   int    `json:"claimed"`
	Left      int    `json:"left"`
	Price     string `json:"price"`
	Area      string `json:"area"`
	Verified  int    `json:"verified"`
	Tier      string `json:"tier"`
	IsPremium int    `json:"is_premium"`
	Timestamp string `json:"timestamp"`
}

type User struct {
	ID          int    `json:"id"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Role        string `json:"role"`
	Tier        string `json:"tier"`
	Status      string `json:"status"`
	RecoveryKey string `json:"recovery_key"`
	Avatar      string `json:"avatar"`
}

type ClaimRecord struct {
	ID        int    `json:"id"`
	Username  string `json:"username"`
	DealID    int    `json:"deal_id"`
	Code      string `json:"code"`
	Content   string `json:"content"`
	Timestamp string `json:"timestamp"`
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

	// Initialize Tables with Category Column
	db.Exec(`CREATE TABLE IF NOT EXISTS jhapats (
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
		user TEXT, handle TEXT, content TEXT, category TEXT, avatar TEXT, 
		image TEXT, timer TEXT, claimed INTEGER, 
		left INTEGER, price TEXT, area TEXT, 
		verified INTEGER DEFAULT 0,
		tier TEXT,
		is_premium INTEGER DEFAULT 0, timestamp TEXT)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT, 
		username TEXT UNIQUE, password TEXT, role TEXT,
		tier TEXT DEFAULT 'free',
		status TEXT DEFAULT 'approved',
		recovery_key TEXT,
		avatar TEXT)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS claims (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT, deal_id INTEGER, code TEXT, timestamp TEXT)`)

	seedData()

	app := fiber.New(fiber.Config{
		BodyLimit: 10 * 1024 * 1024,
	})
	app.Use(logger.New())
	app.Static("/", "./public")

	app.Get("/favicon.ico", func(c *fiber.Ctx) error {
		return c.SendStatus(204)
	})

	app.Post("/api/register", func(c *fiber.Ctx) error {
		u := new(User)
		if err := c.BodyParser(u); err != nil {
			return c.Status(400).SendString("Invalid request format")
		}
		if u.Username == u.Password {
			return c.Status(400).SendString("Password cannot be same as username")
		}
		if u.Role == "admin" {
			return c.Status(403).SendString("Unauthorized")
		}
		status := "approved"
		if u.Role == "merchant" {
			status = "pending"
		}
		hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(u.Password), 10)
		recoveryKey := fmt.Sprintf("REC-%d", time.Now().UnixNano()%1000000)

		_, err := db.Exec("INSERT INTO users (username, password, role, tier, status, recovery_key, avatar) VALUES (?, ?, ?, 'free', ?, ?, ?)",
			u.Username, string(hashedPassword), u.Role, status, recoveryKey, u.Avatar)
		if err != nil {
			return c.Status(500).SendString("User already exists")
		}
		return c.JSON(fiber.Map{"status": "registered", "recovery_key": recoveryKey})
	})

app.Post("/api/login", func(c *fiber.Ctx) error {
        u := new(User)
        c.BodyParser(u)

        // SUPERADMIN CHECK - Now hidden from "View Source"
        if u.Username == "superadmin" && u.Password == "jhapat2026" {
            return c.JSON(fiber.Map{"username": "admin", "role": "admin", "tier": "gold", "status": "approved"})
        }

        var dbPass, dbRole, dbStatus, dbAvatar, tier string
        err := db.QueryRow("SELECT password, role, tier, status, avatar FROM users WHERE username = ?", u.Username).
            Scan(&dbPass, &dbRole, &tier, &dbStatus, &dbAvatar)
        if err != nil || bcrypt.CompareHashAndPassword([]byte(dbPass), []byte(u.Password)) != nil {
            return c.Status(401).SendString("Invalid credentials")
        }

        if dbRole == "merchant" && dbStatus != "approved" {
            return c.Status(403).SendString("Account pending approval")
        }
        return c.JSON(fiber.Map{"username": u.Username, "role": dbRole, "tier": tier, "avatar": dbAvatar})
    })

	app.Put("/api/merchant/edit/:id", func(c *fiber.Ctx) error {
        id := c.Params("id")
        t := new(Jhapat)
        c.BodyParser(t)
        // Update specific fields: Content, Price, Units Left (left), and Category
        _, err := db.Exec("UPDATE jhapats SET content = ?, price = ?, left = ?, category = ? WHERE id = ?", 
            t.Content, t.Price, t.Left, t.Category, id)
        if err != nil { return c.Status(500).SendString("Update failed") }
        return c.SendStatus(200)
    })
	
	app.Get("/api/tweets", func(c *fiber.Ctx) error {
		rows, err := db.Query("SELECT * FROM jhapats ORDER BY is_premium DESC, id DESC")
		if err != nil {
			return c.Status(500).SendString("Could not fetch feed")
		}
		defer rows.Close()

		var feed []Jhapat
		for rows.Next() {
			var t Jhapat
			err := rows.Scan(&t.ID, &t.User, &t.Handle, &t.Content, &t.Category, &t.Avatar, &t.Image, &t.Timer, &t.Claimed, &t.Left, &t.Price, &t.Area, &t.Verified, &t.Tier, &t.IsPremium, &t.Timestamp)
			if err != nil {
				continue
			}
			feed = append(feed, t)
		}
		return c.JSON(feed)
	})

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
			rows.Scan(&t.ID, &t.User, &t.Handle, &t.Content, &t.Category, &t.Avatar, &t.Image, &t.Timer, &t.Claimed, &t.Left, &t.Price, &t.Area, &t.Verified, &t.Tier, &t.IsPremium, &t.Timestamp)
			feed = append(feed, t)
		}
		return c.JSON(feed)
	})

	app.Post("/api/jhapat/claim/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		type ClaimReq struct {
			Username string `json:"username"`
			Code     string `json:"code"`
		}
		req := new(ClaimReq)
		c.BodyParser(req)

		tx, _ := db.Begin()
		_, err := tx.Exec("UPDATE jhapats SET left = left - 1, claimed = claimed + 5 WHERE id = ? AND left > 0", id)
		if err != nil {
			tx.Rollback()
			return c.Status(500).SendString("Could not claim deal")
		}

		_, err = tx.Exec("INSERT INTO claims (username, deal_id, code, timestamp) VALUES (?, ?, ?, ?)",
			req.Username, id, req.Code, time.Now().Format(time.RFC3339))

		tx.Commit()
		return c.SendStatus(200)
	})

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

	app.Post("/api/merchant/deal", func(c *fiber.Ctx) error {
		t := new(Jhapat)
		if err := c.BodyParser(t); err != nil {
			return c.Status(400).SendString("Invalid deal data")
		}
		var tier string
		var avatar string
		db.QueryRow("SELECT tier, avatar FROM users WHERE username = ?", t.User).Scan(&tier, &avatar)

		now := time.Now().Format(time.RFC3339)
		isPremium := 0
		if tier == "gold" {
			isPremium = 1
		}

		// Updated INSERT with category field
		_, err := db.Exec(`INSERT INTO jhapats 
			(user, handle, content, category, avatar, image, timer, claimed, left, price, area, verified, tier, is_premium, timestamp) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ? )`,
			t.User, "@merchant", t.Content, t.Category, avatar, t.Image, t.Timer, 0, t.Left, t.Price, t.Area, 1, tier, isPremium, now)

		if err != nil {
			return c.Status(500).SendString("Could not post deal")
		}
		return c.SendStatus(201)
	})

	app.Get("/api/admin/users", func(c *fiber.Ctx) error {
		rows, _ := db.Query("SELECT id, username, role, tier, status FROM users WHERE role = 'merchant'")
		defer rows.Close()
		var users []User
		for rows.Next() {
			var u User
			rows.Scan(&u.ID, &u.Username, &u.Role, &u.Tier, &u.Status)
			users = append(users, u)
		}
		return c.JSON(users)
	})

	app.Post("/api/admin/approve/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		_, err := db.Exec("UPDATE users SET status = 'approved' WHERE id = ?", id)
		if err != nil {
			return c.Status(500).SendString("Failed to approve merchant")
		}
		return c.SendStatus(200)
	})

	app.Post("/api/admin/update-tier", func(c *fiber.Ctx) error {
		type Update struct {
			Username string `json:"username"`
			Tier string `json:"tier"`
		}
		u := new(Update)
		if err := c.BodyParser(u); err != nil {
			return err
		}
		_, err := db.Exec("UPDATE users SET tier = ? WHERE username = ?", u.Tier, u.Username)
		if err != nil {
			return err
		}
		isPremium := 0
		if u.Tier == "gold" {
			isPremium = 1
		}
		db.Exec("UPDATE jhapats SET is_premium = ?, tier = ? WHERE user = ?", isPremium, u.Tier, u.Username)
		return c.SendStatus(200)
	})

	app.Delete("/api/admin/delete-jhapat/:id", func(c *fiber.Ctx) error {
		id := c.Params("id")
		_, err := db.Exec("DELETE FROM jhapats WHERE id = ?", id)
		if err != nil {
			return c.Status(500).SendString("Failed to delete")
		}
		return c.SendStatus(200)
	})

	app.Post("/api/admin/seed", func(c *fiber.Ctx) error {
		now := time.Now().Format(time.RFC3339)
		db.Exec(`INSERT INTO jhapats (user, handle, content, category, avatar, image, timer, claimed, left, price, area, verified, tier, is_premium, timestamp) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"The Falcon Grill", "@falcon", "50% Off Signature Wings", "Food", "https://i.pravatar.cc/150?u=falcon", "", "2h", 25, 10, "Rs 12.00", "Banjara Hills", 1, "gold", 1, now)
		return c.SendStatus(200)
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	log.Fatal(app.Listen(":" + port))
}

func seedData() {
	adminPass, _ := bcrypt.GenerateFromPassword([]byte("admin123"), 10)
	db.Exec("INSERT OR IGNORE INTO users (username, password, role, tier, status) VALUES (?, ?, ?, 'free', 'approved')", "admin", string(adminPass), "admin")
	var count int
	db.QueryRow("SELECT COUNT(*) FROM jhapats").Scan(&count)
	if count < 2 {
		now := time.Now().Format(time.RFC3339)
		// Added seed data with category
		db.Exec(`INSERT INTO jhapats (user, handle, content, category, avatar, image, timer, claimed, left, price, area, verified, tier, is_premium, timestamp) 
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			"The Falcon Grill", "@falcon", "50% Off Signature Wings", "Food", "https://i.pravatar.cc/150?u=falcon", "", "2h", 25, 10, "Rs 12.00", "Banjara Hills", 1, "gold", 1, now)
	}
}
