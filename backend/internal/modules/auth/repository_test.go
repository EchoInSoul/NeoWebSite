package auth

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/config"
	"website/backend/internal/database"
)

func testDB(t *testing.T) *database.Database {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("SKIP: DATABASE_URL_TEST is not set")
	}

	db, err := database.Open(context.Background(), config.Config{
		DatabaseURL:       url,
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func uniqueTag(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func createTestUser(t *testing.T, db *database.Database, tag string) (User, string) {
	t.Helper()

	password := "pass-" + tag
	u, err := CreateAdminUser(context.Background(), db, "Admin+"+tag+"@Example.COM", "nick"+tag, password)
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.User.DeleteOneID(uuid.MustParse(u.ID)).Exec(context.Background())
	})
	return u, password
}

func TestCreateAdminUserAndFindByIdentifier(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	repo := newRepository(db)

	byEmail, err := repo.findUserByIdentifier(context.Background(), "admin+"+tag+"@example.com")
	if err != nil {
		t.Fatalf("find by lowercase email: %v", err)
	}
	if byEmail.ID.String() != u.ID {
		t.Fatalf("id = %s, want %s", byEmail.ID, u.ID)
	}

	byUpper, err := repo.findUserByIdentifier(context.Background(), "ADMIN+"+tag+"@EXAMPLE.COM")
	if err != nil {
		t.Fatalf("find by uppercase email: %v", err)
	}
	if byUpper.ID.String() != u.ID {
		t.Fatalf("upper id = %s, want %s", byUpper.ID, u.ID)
	}

	byNickname, err := repo.findUserByIdentifier(context.Background(), "nick"+tag)
	if err != nil {
		t.Fatalf("find by nickname: %v", err)
	}
	if byNickname.ID.String() != u.ID {
		t.Fatalf("nickname id = %s, want %s", byNickname.ID, u.ID)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(byEmail.PasswordHash), []byte(password)); err != nil {
		t.Fatalf("stored hash does not verify: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(byEmail.PasswordHash), []byte(password+"-wrong")); err == nil {
		t.Fatal("expected wrong password to fail verification")
	}
}

func TestFindUserByID(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, _ := createTestUser(t, db, tag)
	repo := newRepository(db)

	found, err := repo.findUserByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("find by id: %v", err)
	}
	if found.Email != u.Email {
		t.Fatalf("email = %s, want %s", found.Email, u.Email)
	}
}

func TestFindUserByIDNotFound(t *testing.T) {
	db := testDB(t)
	repo := newRepository(db)

	missing := uuid.Must(uuid.NewV7()).String()
	if _, err := repo.findUserByID(context.Background(), missing); !errors.Is(err, errUserNotFound) {
		t.Fatalf("missing id: err = %v, want errUserNotFound", err)
	}
	if _, err := repo.findUserByID(context.Background(), "not-a-uuid"); !errors.Is(err, errUserNotFound) {
		t.Fatalf("malformed id: err = %v, want errUserNotFound", err)
	}
}

func TestCreateAdminUserRejectsDuplicates(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	createTestUser(t, db, tag)

	_, err := CreateAdminUser(context.Background(), db, "admin+"+tag+"@example.com", "other"+tag, "pw123456")
	if err == nil {
		t.Fatal("expected duplicate email to fail")
	}
	if errors.Is(err, errUserNotFound) {
		t.Fatal("duplicate email must not be reported as errUserNotFound")
	}

	_, err = CreateAdminUser(context.Background(), db, "other+"+tag+"@example.com", "nick"+tag, "pw123456")
	if err == nil {
		t.Fatal("expected duplicate nickname to fail")
	}
	if errors.Is(err, errUserNotFound) {
		t.Fatal("duplicate nickname must not be reported as errUserNotFound")
	}
}

func TestCreateAdminUserRejectsLongPassword(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)

	_, err := CreateAdminUser(context.Background(), db, "long+"+tag+"@example.com", "long"+tag, strings.Repeat("a", 73))
	if err == nil {
		t.Fatal("expected 73-byte password to be rejected")
	}
}

func TestCreateAdminUserRejectsEmptyPassword(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)

	created, err := CreateAdminUser(context.Background(), db, "empty+"+tag+"@example.com", "empty"+tag, "")
	if err == nil {
		t.Cleanup(func() {
			_ = db.Client.User.DeleteOneID(uuid.MustParse(created.ID)).Exec(context.Background())
		})
		t.Fatal("expected empty password to be rejected")
	}
}
