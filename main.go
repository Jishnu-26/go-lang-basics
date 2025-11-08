package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Book struct {
	ID       int    `json:"id"`
	Title    string `json:"title"`
	Author   string `json:"author"`
	Quantity int    `json:"quantity"`
}

var dbPool *pgxpool.Pool

func initDB() {
	host := getEnv("DB_HOST", "localhost")
	port := getEnv("DB_PORT", "5432")
	user := getEnv("DB_USER", "bookuser")
	password := getEnv("DB_PASSWORD", "bookpass")
	dbname := getEnv("DB_NAME", "bookstore")

	connString := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user, password, host, port, dbname)

	var err error
	dbPool, err = pgxpool.New(context.Background(), connString)
	if err != nil {
		log.Fatal("Unable to create connection pool:", err)
	}

	err = dbPool.Ping(context.Background())
	if err != nil {
		log.Fatal("Unable to ping database:", err)
	}

	log.Println("Successfully connected to database!")

	createTable()

	seedData()
}

func createTable() {
	query := `
    CREATE TABLE IF NOT EXISTS books (
        id SERIAL PRIMARY KEY,
        title VARCHAR(255) NOT NULL,
        author VARCHAR(255) NOT NULL,
        quantity INTEGER NOT NULL DEFAULT 0
    )`

	_, err := dbPool.Exec(context.Background(), query)
	if err != nil {
		log.Fatal("Failed to create table:", err)
	}
	log.Println("Table created or already exists")
}

func seedData() {
	var count int
	err := dbPool.QueryRow(context.Background(), "SELECT COUNT(*) FROM books").Scan(&count)
	if err != nil {
		log.Println("Error checking data:", err)
		return
	}

	if count == 0 {
		query := `
        INSERT INTO books (title, author, quantity) VALUES
            ('The Great Gatsby', 'F. Scott Fitzgerald', 3),
            ('1984', 'George Orwell', 5),
            ('To Kill a Mockingbird', 'Harper Lee', 4)
        `
		_, err := dbPool.Exec(context.Background(), query)
		if err != nil {
			log.Println("Error seeding data:", err)
			return
		}
		log.Println("Initial data seeded")
	}
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getBooks(c *gin.Context) {
	query := "SELECT id, title, author, quantity FROM books ORDER BY id"
	rows, err := dbPool.Query(context.Background(), query)
	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "database error"})
		log.Println("Error querying books:", err)
		return
	}
	defer rows.Close()

	var books []Book
	for rows.Next() {
		var book Book
		err := rows.Scan(&book.ID, &book.Title, &book.Author, &book.Quantity)
		if err != nil {
			c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "error scanning data"})
			log.Println("Error scanning row:", err)
			return
		}
		books = append(books, book)
	}

	if err = rows.Err(); err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "error iterating rows"})
		return
	}

	c.IndentedJSON(http.StatusOK, books)
}

func booksByID(c *gin.Context) {
	id := c.Param("id")
	bookID, err := strconv.Atoi(id)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": "invalid book ID"})
		return
	}

	query := "SELECT id, title, author, quantity FROM books WHERE id = $1"
	var book Book
	err = dbPool.QueryRow(context.Background(), query, bookID).Scan(
		&book.ID, &book.Title, &book.Author, &book.Quantity)

	if err != nil {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "book not found"})
		return
	}

	c.IndentedJSON(http.StatusOK, book)
}

func addBook(c *gin.Context) {
	var newBook Book

	if err := c.BindJSON(&newBook); err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": "invalid request"})
		return
	}

	query := `
        INSERT INTO books (title, author, quantity) 
        VALUES ($1, $2, $3) 
        RETURNING id`

	err := dbPool.QueryRow(context.Background(), query,
		newBook.Title, newBook.Author, newBook.Quantity).Scan(&newBook.ID)

	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "failed to add book"})
		log.Println("Error adding book:", err)
		return
	}

	c.IndentedJSON(http.StatusCreated, newBook)
}

func updateBook(c *gin.Context) {
	id := c.Param("id")
	bookID, err := strconv.Atoi(id)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": "invalid book ID"})
		return
	}

	var book Book
	if err := c.BindJSON(&book); err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": "invalid request"})
		return
	}

	query := `
        UPDATE books 
        SET title = $1, author = $2, quantity = $3 
        WHERE id = $4`

	result, err := dbPool.Exec(context.Background(), query,
		book.Title, book.Author, book.Quantity, bookID)

	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "failed to update book"})
		log.Println("Error updating book:", err)
		return
	}

	if result.RowsAffected() == 0 {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "book not found"})
		return
	}

	book.ID = bookID
	c.IndentedJSON(http.StatusOK, book)
}

func deleteBook(c *gin.Context) {
	id := c.Param("id")
	bookID, err := strconv.Atoi(id)
	if err != nil {
		c.IndentedJSON(http.StatusBadRequest, gin.H{"message": "invalid book ID"})
		return
	}

	query := "DELETE FROM books WHERE id = $1"
	result, err := dbPool.Exec(context.Background(), query, bookID)

	if err != nil {
		c.IndentedJSON(http.StatusInternalServerError, gin.H{"message": "failed to delete book"})
		log.Println("Error deleting book:", err)
		return
	}

	if result.RowsAffected() == 0 {
		c.IndentedJSON(http.StatusNotFound, gin.H{"message": "book not found"})
		return
	}

	c.IndentedJSON(http.StatusOK, gin.H{"message": "book deleted successfully"})
}

func main() {
	initDB()
	defer dbPool.Close()

	router := gin.Default()

	router.GET("/books", getBooks)
	router.GET("/books/:id", booksByID)
	router.POST("/books", addBook)
	router.PUT("/books/:id", updateBook)
	router.DELETE("/books/:id", deleteBook)

	log.Println("Server starting on :8080")
	err := router.Run("0.0.0.0:8080")
	if err != nil {
		return
	}
}
