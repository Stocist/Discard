package database

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Stocist/discard/internal/models"
	"github.com/google/uuid"
)

func TestMessageCreateRejectsBlockedDMInsideTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	channelID := uuid.New()
	authorID := uuid.New()
	otherID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT server_id FROM channels").WithArgs(channelID).
		WillReturnRows(sqlmock.NewRows([]string{"server_id"}).AddRow(nil))
	mock.ExpectQuery("SELECT user_id FROM dm_members").WithArgs(channelID, authorID).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(otherID))
	mock.ExpectQuery("SELECT id FROM users").WithArgs(authorID, otherID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(authorID).AddRow(otherID))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(authorID, otherID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	err = (&MessageRepo{DB: db}).Create(context.Background(), &models.Message{
		ChannelID: channelID,
		AuthorID:  authorID,
		Content:   "blocked",
	})
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("Create error = %v, want ErrBlocked", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMessageUpdateRejectsBlockedDMInsideTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	messageID := uuid.New()
	channelID := uuid.New()
	authorID := uuid.New()
	otherID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT channel_id FROM messages").WithArgs(messageID, authorID).
		WillReturnRows(sqlmock.NewRows([]string{"channel_id"}).AddRow(channelID))
	mock.ExpectQuery("SELECT server_id FROM channels").WithArgs(channelID).
		WillReturnRows(sqlmock.NewRows([]string{"server_id"}).AddRow(nil))
	mock.ExpectQuery("SELECT user_id FROM dm_members").WithArgs(channelID, authorID).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow(otherID))
	mock.ExpectQuery("SELECT id FROM users").WithArgs(authorID, otherID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(authorID).AddRow(otherID))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(authorID, otherID).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	_, err = (&MessageRepo{DB: db}).Update(context.Background(), messageID, authorID, "blocked edit")
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("Update error = %v, want ErrBlocked", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestBlockLocksUserPairBeforeInsert(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	blockerID := uuid.New()
	blockedID := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM users").WithArgs(blockerID, blockedID).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(blockerID).AddRow(blockedID))
	mock.ExpectExec("INSERT INTO user_blocks").WithArgs(blockerID, blockedID).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := (&BlockRepo{DB: db}).Block(context.Background(), blockerID, blockedID); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestOpenDMRejectsBlockedPairInsideTransaction(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	userA := uuid.New()
	userB := uuid.New()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT id FROM users").WithArgs(userA, userB).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(userA).AddRow(userB))
	mock.ExpectQuery("SELECT c.id").WithArgs(userA, userB).
		WillReturnRows(sqlmock.NewRows([]string{"id", "server_id", "name", "topic", "type", "position", "created_at"}))
	mock.ExpectQuery("SELECT EXISTS").WithArgs(userA, userB).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	mock.ExpectRollback()

	_, _, err = (&DMMemberRepo{DB: db}).OpenDM(context.Background(), userA, userB)
	if !errors.Is(err, ErrBlocked) {
		t.Fatalf("OpenDM error = %v, want ErrBlocked", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
