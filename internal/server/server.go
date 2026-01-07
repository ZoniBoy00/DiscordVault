package server

import (
	"bytes"
	"crypto/sha256"
	"discordvault/internal/bot"
	"discordvault/internal/config"
	"discordvault/internal/crypto"
	"discordvault/internal/database"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

type Server struct {
	Config *config.Config
	DB     *database.Database
	Bot    *bot.Bot
}

func New(cfg *config.Config, db *database.Database, vaultBot *bot.Bot) *Server {
	return &Server{
		Config: cfg,
		DB:     db,
		Bot:    vaultBot,
	}
}

func (s *Server) Start() error {
	r := mux.NewRouter()

	// API Endpoints
	r.HandleFunc("/api/upload", s.handleUpload).Methods("POST")
	r.HandleFunc("/api/files", s.handleListFiles).Methods("GET")
	r.HandleFunc("/api/download/{id}", s.handleDownload).Methods("GET")
	r.HandleFunc("/api/delete/{id}", s.handleDelete).Methods("POST")

	// Static Assets
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./web/")))

	srv := &http.Server{
		Handler:      r,
		Addr:         ":8080",
		WriteTimeout: 0,
		ReadTimeout:  0,
	}

	log.Printf("[SERVER] Neural Link Established at http://localhost:8080")
	return srv.ListenAndServe()
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	files, err := s.DB.ListFiles()
	if err != nil {
		log.Printf("[SRV ERR] ListFiles failed: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])

	chunks, err := s.DB.GetChunks(id)
	if err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	log.Printf("[SERVER] Initiating parallel wipe for File ID: %d (%d chunks)", id, len(chunks))

	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 8) // Parallel delete workers

	for _, chunk := range chunks {
		wg.Add(1)
		go func(msgID string) {
			defer wg.Done()
			semaphore <- struct{}{}
			_ = s.Bot.Session.ChannelMessageDelete(s.Config.ChannelID, msgID)
			<-semaphore
		}(chunk.MessageID)
	}
	wg.Wait()

	if err := s.DB.DeleteFile(id); err != nil {
		log.Printf("[SRV ERR] Metadata purge failed: %v", err)
		http.Error(w, "Registry purge failed", http.StatusInternalServerError)
		return
	}

	log.Printf("[SERVER] File ID %d successfully erased from cluster.", id)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	mr, err := r.MultipartReader()
	if err != nil {
		http.Error(w, "Stream initialization failed", http.StatusBadRequest)
		return
	}

	var filename string
	var totalSize int64
	var messageIDs []string
	hasher := sha256.New()

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if part.FormName() == "file" {
			filename = part.FileName()
			buffer := make([]byte, bot.ChunkSize)
			partNum := 1

			log.Printf("[SERVER] Receiving transmission: %s", filename)

			for {
				n, err := io.ReadFull(part, buffer)
				if n > 0 {
					chunkData := buffer[:n]
					totalSize += int64(n)
					hasher.Write(chunkData)

					// Encrypt payload
					encrypted, err := crypto.Encrypt(chunkData, s.Config.EncryptionKey)
					if err != nil {
						log.Printf("[SRV ERR] Encryption failed: %v", err)
						http.Error(w, "Security fault", http.StatusInternalServerError)
						return
					}

					// Sent to Discord storage
					msg, err := s.Bot.Session.ChannelFileSend(s.Config.ChannelID, fmt.Sprintf("%x.vault", sha256.Sum256(encrypted)), bytes.NewReader(encrypted))
					if err != nil {
						log.Printf("[SRV ERR] Discord rejection at chunk %d: %v", partNum, err)
						http.Error(w, "Decentralized storage rejection", http.StatusInternalServerError)
						return
					}

					messageIDs = append(messageIDs, msg.ID)
					log.Printf("[SERVER] Chunk %d secured (%d bytes)", partNum, len(encrypted))
					partNum++

					// Rate limit protection
					time.Sleep(800 * time.Millisecond)
				}
				if err == io.EOF || err == io.ErrUnexpectedEOF {
					break
				}
			}
		}
	}

	if len(messageIDs) == 0 {
		http.Error(w, "Payload empty", http.StatusBadRequest)
		return
	}

	hashStr := hex.EncodeToString(hasher.Sum(nil))
	fileID, err := s.DB.SaveFile(filename, totalSize, hashStr)
	if err == nil {
		for idx, msgID := range messageIDs {
			s.DB.SaveChunk(fileID, msgID, idx+1)
		}
		go s.Bot.NotifyUpload(filename, totalSize, len(messageIDs), "Web")
	}

	log.Printf("[SERVER] Transmission complete: %s (ID: #%d)", filename, fileID)
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id, _ := strconv.Atoi(vars["id"])

	file, err := s.DB.GetFile(id)
	if err != nil {
		http.Error(w, "Object not found", http.StatusNotFound)
		return
	}

	chunks, _ := s.DB.GetChunks(id)

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", file.Name))
	w.Header().Set("Content-Type", "application/octet-stream")

	log.Printf("[SERVER] Reconstructing object: %s", file.Name)

	for _, chunk := range chunks {
		msg, err := s.Bot.Session.ChannelMessage(s.Config.ChannelID, chunk.MessageID)
		if err != nil || len(msg.Attachments) == 0 {
			log.Printf("[SRV ERR] Fragment missing: %d", chunk.PartNum)
			continue
		}

		resp, err := http.Get(msg.Attachments[0].URL)
		if err != nil {
			log.Printf("[SRV ERR] Fragment fetch failed: %v", err)
			continue
		}

		encrypted, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		decrypted, err := crypto.Decrypt(encrypted, s.Config.EncryptionKey)
		if err != nil {
			log.Printf("[SRV ERR] Decryption fault at chunk %d: %v", chunk.PartNum, err)
			return
		}

		w.Write(decrypted)
	}
	log.Printf("[SERVER] Object %s successfully delivered.", file.Name)
}
