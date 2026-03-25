package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"
	pencrypto "github.com/seastco/penpal/internal/crypto"
)

type RouteHop struct {
	City  string    `json:"city"`
	Code  string    `json:"code"`
	Relay string    `json:"relay"`
	Lat   float64   `json:"lat"`
	Lng   float64   `json:"lng"`
	ETA   time.Time `json:"eta"`
}

type cityInfo struct {
	name string
	lat  float64
	lng  float64
}

var cities = []cityInfo{
	{"New York, NY", 40.7128, -74.0060},
	{"Los Angeles, CA", 34.0522, -118.2437},
	{"Chicago, IL", 41.8781, -87.6298},
	{"Houston, TX", 29.7604, -95.3698},
	{"Phoenix, AZ", 33.4484, -112.0740},
	{"Philadelphia, PA", 39.9526, -75.1652},
	{"San Antonio, TX", 29.4241, -98.4936},
	{"San Diego, CA", 32.7157, -117.1611},
	{"Dallas, TX", 32.7767, -96.7970},
	{"Austin, TX", 30.2672, -97.7431},
	{"Jacksonville, FL", 30.3322, -81.6557},
	{"San Jose, CA", 37.3382, -121.8863},
	{"Fort Worth, TX", 32.7555, -97.3308},
	{"Columbus, OH", 39.9612, -82.9988},
	{"Charlotte, NC", 35.2271, -80.8431},
	{"Indianapolis, IN", 39.7684, -86.1581},
	{"San Francisco, CA", 37.7749, -122.4194},
	{"Seattle, WA", 47.6062, -122.3321},
	{"Denver, CO", 39.7392, -104.9903},
	{"Nashville, TN", 36.1627, -86.7816},
	{"Oklahoma City, OK", 35.4676, -97.5164},
	{"El Paso, TX", 31.7619, -106.4850},
	{"Portland, OR", 45.5152, -122.6784},
	{"Las Vegas, NV", 36.1699, -115.1398},
	{"Memphis, TN", 35.1495, -90.0490},
	{"Louisville, KY", 38.2527, -85.7585},
	{"Baltimore, MD", 39.2904, -76.6122},
	{"Milwaukee, WI", 43.0389, -87.9065},
	{"Albuquerque, NM", 35.0844, -106.6504},
	{"Tucson, AZ", 32.2226, -110.9747},
	{"Fresno, CA", 36.7378, -119.7871},
	{"Sacramento, CA", 38.5816, -121.4944},
	{"Mesa, AZ", 33.4152, -111.8315},
	{"Kansas City, MO", 39.0997, -94.5786},
	{"Atlanta, GA", 33.7490, -84.3880},
	{"Omaha, NE", 41.2565, -95.9345},
	{"Raleigh, NC", 35.7796, -78.6382},
	{"Miami, FL", 25.7617, -80.1918},
	{"Minneapolis, MN", 44.9778, -93.2650},
	{"Cleveland, OH", 41.4993, -81.6944},
	{"Tampa, FL", 27.9506, -82.4572},
	{"New Orleans, LA", 29.9511, -90.0715},
	{"Pittsburgh, PA", 40.4406, -79.9959},
	{"Cincinnati, OH", 39.1031, -84.5120},
	{"St. Louis, MO", 38.6270, -90.1994},
	{"Orlando, FL", 28.5383, -81.3792},
	{"Detroit, MI", 42.3314, -83.0458},
	{"Salt Lake City, UT", 40.7608, -111.8910},
	{"Richmond, VA", 37.5407, -77.4360},
	{"Boise, ID", 43.6150, -116.2023},
	{"Anchorage, AK", 61.2181, -149.9003},
	{"Honolulu, HI", 21.3069, -157.8583},
	{"Madrid, ES", 40.4168, -3.7038},
}

var names = []string{
	"alice", "bob", "carol", "dave", "eve",
	"frank", "grace", "hank", "iris", "jack",
	"kate", "leo", "mia", "nick", "olivia",
	"pete", "quinn", "rosa", "sam", "tara",
	"uma", "vince", "wendy", "xander", "yara",
	"zach", "amber", "blake", "chloe", "derek",
	"ellie", "finn", "gina", "hugo", "isla",
	"james", "kira", "liam", "maya", "nolan",
	"opal", "paul", "rae", "seth", "tess",
	"uri", "val", "wade", "xena", "yuki",
}

var letterBodies = []string{
	"Hey! Just wanted to say hi from across the country. How's everything going over there?",
	"I saw the most beautiful sunset today. Wish you could have seen it. The sky turned this incredible shade of pink and orange.",
	"Remember that book you recommended? I finally finished it. You were right — the ending was completely unexpected. We need to talk about it!",
	"Happy birthday! I know this letter might arrive a few days late, but I hope you had an amazing day. Here's to another great year!",
	"I've been thinking about our road trip last summer. We should definitely do it again. Maybe head out west this time?",
	"Quick update: I got the job! Starting next month. Thanks for all the encouragement — it really helped me push through the interviews.",
	"The weather here has been absolutely wild. Snow one day, sunshine the next. I can never decide what to wear anymore.",
	"Found this amazing coffee shop downtown. They roast their own beans and the espresso is incredible. You'd love it.",
	"I started learning guitar! My fingers hurt but I can almost play a full song now. Don't laugh when you hear it though.",
	"Just moved into my new apartment. Still living out of boxes but the view from the balcony is worth it. Come visit!",
	"Do you remember Mrs. Patterson from school? I ran into her at the grocery store. She asked about you!",
	"I tried making that pasta recipe you sent me. It turned out... interesting. I think I added too much garlic. Is that even possible?",
	"Been going on long walks in the morning lately. There's something peaceful about a city before everyone wakes up.",
	"My cat did the funniest thing today — she tried to catch a bird through the window and knocked over my entire bookshelf.",
	"I know things have been tough lately. Just wanted you to know I'm thinking of you and I'm here if you need anything.",
	"The autumn leaves here are incredible this year. Reds, oranges, yellows everywhere. I've been taking so many photos.",
	"Guess what? I'm going to be an aunt/uncle! My sibling just told us the news. Everyone is so excited!",
	"I picked up painting last month. Everything I make looks like abstract art whether I intend it to or not. At least it's fun.",
	"There's a new farmer's market near my place. Fresh bread, local honey, handmade soap — the works. Saturday mornings are the best now.",
	"I finally watched that show you've been telling me about for months. You were right. I binged the entire season in two days.",
	"Writing to you from a train! I'm heading upstate for the weekend. The scenery passing by the window is gorgeous.",
	"I've been volunteering at the local shelter on weekends. The dogs there are so sweet. Almost adopted three of them.",
	"Do you still collect vinyl records? I found a rare one at a thrift shop and immediately thought of you. Sending it your way!",
	"My garden is finally producing tomatoes! After months of watering and worrying, I actually grew food. Feeling very accomplished.",
	"I had the weirdest dream last night — you were in it and we were flying kites on the moon. My brain is strange.",
	"Just finished a marathon! Well, a half marathon. Okay, a 5K. But I finished it and that's what counts!",
	"The neighbors got a puppy and it keeps escaping into my yard. I'm not mad about it. Best part of my day honestly.",
	"I'm reading this fascinating book about the history of letters and postal systems. Made me appreciate what we're doing here even more.",
	"Made homemade ice cream for the first time. Vanilla bean with chocolate chunks. It's dangerously good. I might never buy store-bought again.",
	"There's a meteor shower happening next week. Going to drive out to the countryside to watch. Stars are impossible to see from the city.",
}

func code(city string) string {
	if len(city) < 3 {
		return "xxx"
	}
	return fmt.Sprintf("%.3s", city)
}

func makeRoute(from, to cityInfo, mid *cityInfo, sentAt, releaseAt time.Time) []RouteHop {
	hops := []RouteHop{
		{City: from.name, Code: code(from.name), Relay: code(from.name) + "-relay-01", Lat: from.lat, Lng: from.lng, ETA: sentAt},
	}
	if mid != nil {
		midETA := sentAt.Add(releaseAt.Sub(sentAt) / 2)
		hops = append(hops, RouteHop{City: mid.name, Code: code(mid.name), Relay: code(mid.name) + "-relay-01", Lat: mid.lat, Lng: mid.lng, ETA: midETA})
	}
	hops = append(hops, RouteHop{City: to.name, Code: code(to.name), Relay: code(to.name) + "-relay-01", Lat: to.lat, Lng: to.lng, ETA: releaseAt})
	return hops
}

func main() {
	userFlag := flag.String("user", "seastco#5253", "target user in name#disc format")
	flag.Parse()

	parts := strings.SplitN(*userFlag, "#", 2)
	if len(parts) != 2 {
		log.Fatalf("invalid user format %q, expected name#disc", *userFlag)
	}
	targetName, targetDisc := parts[0], parts[1]

	dbURL := os.Getenv("PENPAL_DB")
	if dbURL == "" {
		dbURL = "postgres://localhost:5432/penpal?sslmode=disable"
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// Get target user's public key
	var targetPubHex []byte
	var targetID uuid.UUID
	err = db.QueryRowContext(ctx, "SELECT id, public_key FROM users WHERE username=$1 AND discriminator=$2", targetName, targetDisc).Scan(&targetID, &targetPubHex)
	if err != nil {
		log.Fatalf("can't find %s#%s: %v", targetName, targetDisc, err)
	}
	targetPub := ed25519.PublicKey(targetPubHex)
	fmt.Printf("%s#%s ID: %s, pubkey: %x\n", targetName, targetDisc, targetID, targetPub[:8])

	// Clean up previous seed data (seed users have discriminators 1001-1050)
	fmt.Println("Cleaning previous seed data...")
	db.ExecContext(ctx, "DELETE FROM messages WHERE sender_id IN (SELECT id FROM users WHERE discriminator >= '1001' AND discriminator <= '1050') OR recipient_id IN (SELECT id FROM users WHERE discriminator >= '1001' AND discriminator <= '1050')")
	db.ExecContext(ctx, "DELETE FROM contacts WHERE contact_id IN (SELECT id FROM users WHERE discriminator >= '1001' AND discriminator <= '1050')")
	db.ExecContext(ctx, "DELETE FROM users WHERE discriminator >= '1001' AND discriminator <= '1050'")
	// Also clean old seed messages sent TO seastco with fake bodies
	db.ExecContext(ctx, "DELETE FROM messages WHERE encrypted_body = '\\x00deadbeef'")

	tiers := []string{"standard", "priority", "express"}

	type peer struct {
		id   uuid.UUID
		name string
		city cityInfo
		pub  ed25519.PublicKey
		priv ed25519.PrivateKey
	}

	// Create 50 peers with real keypairs
	peers := make([]peer, 50)
	for i := 0; i < 50; i++ {
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			log.Fatal(err)
		}

		disc := fmt.Sprintf("%03d", 101+i)
		city := cities[i]

		var id uuid.UUID
		err = db.QueryRowContext(ctx,
			`INSERT INTO users (id, username, discriminator, public_key, home_city, home_lat, home_lng, last_active, created_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, NOW(), NOW() - interval '30 days')
			 ON CONFLICT (username, discriminator) DO UPDATE SET public_key = $3, last_active = NOW()
			 RETURNING id`,
			names[i], disc, []byte(pub), city.name, city.lat, city.lng,
		).Scan(&id)
		if err != nil {
			log.Fatalf("creating user %s#%s: %v", names[i], disc, err)
		}

		peers[i] = peer{id: id, name: names[i], city: city, pub: pub, priv: priv}

		// Add as seastco's contact
		_, err = db.ExecContext(ctx,
			`INSERT INTO contacts (id, owner_id, contact_id, created_at)
			 VALUES (gen_random_uuid(), $1, $2, NOW() - $3::interval)
			 ON CONFLICT (owner_id, contact_id) DO NOTHING`,
			targetID, id, fmt.Sprintf("%d days", 50-i))
		if err != nil {
			log.Fatalf("adding contact: %v", err)
		}
	}
	fmt.Printf("Created %d peer users with real keypairs\n", len(peers))

	// 100 RECEIVED letters (peer -> seastco, encrypted with peer's private key + seastco's public key)
	received := 0
	for i := 0; i < 100; i++ {
		p := peers[i%50]
		tier := tiers[i%3]
		body := letterBodies[i%len(letterBodies)]
		msgID := uuid.New()

		// Encrypt: sender=peer, recipient=seastco
		encrypted, err := pencrypto.Encrypt([]byte(body), p.priv, targetPub)
		if err != nil {
			log.Fatalf("encrypting message %d: %v", i, err)
		}

		mid := cities[(i+25)%50]
		boston := cityInfo{"Boston, MA", 42.357603, -71.068432}

		if i < 5 {
			// In-transit incoming
			sentAt := time.Now().Add(-24*time.Hour + time.Duration(i)*2*time.Hour)
			releaseAt := time.Now().Add(time.Duration(i+1) * 12 * time.Hour)
			route := makeRoute(p.city, boston, &mid, sentAt, releaseAt)
			routeJSON, _ := json.Marshal(route)

			_, err = db.ExecContext(ctx,
				`INSERT INTO messages (id, sender_id, recipient_id, encrypted_body, shipping_tier, route, sent_at, release_at, status)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'in_transit')`,
				msgID, p.id, targetID, encrypted, tier, routeJSON, sentAt, releaseAt)
		} else if i < 30 {
			// Delivered unread
			sentAt := time.Now().Add(-time.Duration(i) * 6 * time.Hour)
			releaseAt := sentAt.Add(2 * 24 * time.Hour)
			route := makeRoute(p.city, boston, &mid, sentAt, releaseAt)
			routeJSON, _ := json.Marshal(route)

			_, err = db.ExecContext(ctx,
				`INSERT INTO messages (id, sender_id, recipient_id, encrypted_body, shipping_tier, route, sent_at, release_at, delivered_at, status)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, 'delivered')`,
				msgID, p.id, targetID, encrypted, tier, routeJSON, sentAt, releaseAt)
		} else {
			// Read
			sentAt := time.Now().Add(-time.Duration(i) * 4 * time.Hour)
			releaseAt := sentAt.Add(24 * time.Hour)
			route := makeRoute(p.city, boston, nil, sentAt, releaseAt)
			routeJSON, _ := json.Marshal(route)

			readAt := releaseAt.Add(time.Hour)
			_, err = db.ExecContext(ctx,
				`INSERT INTO messages (id, sender_id, recipient_id, encrypted_body, shipping_tier, route, sent_at, release_at, delivered_at, read_at, status)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9, 'read')`,
				msgID, p.id, targetID, encrypted, tier, routeJSON, sentAt, releaseAt, readAt)
		}
		if err != nil {
			log.Fatalf("inserting received message %d: %v", i, err)
		}
		received++
	}
	fmt.Printf("Inserted %d received letters (encrypted with real keys)\n", received)

	// 100 SENT letters (seastco -> peer)
	// We don't have seastco's private key, so we need to encrypt with a throwaway
	// key that the peer could decrypt. But since we only care about seastco READING
	// incoming mail, for sent letters we just need valid-looking encrypted data.
	// The sent view doesn't decrypt — it just shows metadata.
	sent := 0
	for i := 0; i < 100; i++ {
		p := peers[i%50]
		tier := tiers[i%3]
		msgID := uuid.New()

		// For sent messages, encrypt with peer's keypair (won't be decrypted by the UI)
		body := letterBodies[(i+15)%len(letterBodies)]
		encrypted, err := pencrypto.Encrypt([]byte(body), p.priv, p.pub)
		if err != nil {
			log.Fatalf("encrypting sent message %d: %v", i, err)
		}

		boston := cityInfo{"Boston, MA", 42.357603, -71.068432}
		mid := cities[(i+17)%50]

		if i < 8 {
			// In-transit outgoing
			sentAt := time.Now().Add(-12*time.Hour + time.Duration(i)*time.Hour)
			releaseAt := time.Now().Add(time.Duration(i+1) * 8 * time.Hour)
			route := makeRoute(boston, p.city, &mid, sentAt, releaseAt)
			routeJSON, _ := json.Marshal(route)

			_, err = db.ExecContext(ctx,
				`INSERT INTO messages (id, sender_id, recipient_id, encrypted_body, shipping_tier, route, sent_at, release_at, status)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'in_transit')`,
				msgID, targetID, p.id, encrypted, tier, routeJSON, sentAt, releaseAt)
		} else if i < 40 {
			// Delivered
			sentAt := time.Now().Add(-time.Duration(i) * 5 * time.Hour)
			releaseAt := sentAt.Add(2 * 24 * time.Hour)
			route := makeRoute(boston, p.city, &mid, sentAt, releaseAt)
			routeJSON, _ := json.Marshal(route)

			_, err = db.ExecContext(ctx,
				`INSERT INTO messages (id, sender_id, recipient_id, encrypted_body, shipping_tier, route, sent_at, release_at, delivered_at, status)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, 'delivered')`,
				msgID, targetID, p.id, encrypted, tier, routeJSON, sentAt, releaseAt)
		} else {
			// Read
			sentAt := time.Now().Add(-time.Duration(i) * 3 * time.Hour)
			releaseAt := sentAt.Add(24 * time.Hour)
			route := makeRoute(boston, p.city, nil, sentAt, releaseAt)
			routeJSON, _ := json.Marshal(route)

			readAt := releaseAt.Add(2 * time.Hour)
			_, err = db.ExecContext(ctx,
				`INSERT INTO messages (id, sender_id, recipient_id, encrypted_body, shipping_tier, route, sent_at, release_at, delivered_at, read_at, status)
				 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $9, 'read')`,
				msgID, targetID, p.id, encrypted, tier, routeJSON, sentAt, releaseAt, readAt)
		}
		if err != nil {
			log.Fatalf("inserting sent message %d: %v", i, err)
		}
		sent++
	}
	fmt.Printf("Inserted %d sent letters\n", sent)

	// Add stamps for variety
	stampCount := 0

	// State stamps
	stateStamps := []string{
		"state:ma", "state:ny", "state:ca", "state:tx", "state:il",
		"state:fl", "state:pa", "state:oh", "state:ga", "state:nc",
		"state:wa", "state:co", "state:tn", "state:or", "state:nv",
	}
	for i, st := range stateStamps {
		_, err := db.ExecContext(ctx,
			`INSERT INTO stamps (id, owner_id, stamp_type, rarity, earned_via, created_at)
			 VALUES (gen_random_uuid(), $1, $2, 'common', 'registration', NOW() - $3::interval)`,
			targetID, st, fmt.Sprintf("%d days", 30-i))
		if err != nil {
			log.Printf("stamp %s: %v", st, err)
		}
		stampCount++
	}

	// Common stamps (some stacked)
	commonStamps := []string{
		"common:flag", "common:heart", "common:star", "common:quill",
		"common:blossom", "common:sunflower", "common:butterfly", "common:wave",
		"common:moon", "common:bird", "common:rainbow", "common:clover",
	}
	for i, st := range commonStamps {
		// Insert 1-3 copies to show stacking
		copies := (i % 3) + 1
		for c := 0; c < copies; c++ {
			_, err := db.ExecContext(ctx,
				`INSERT INTO stamps (id, owner_id, stamp_type, rarity, earned_via, created_at)
				 VALUES (gen_random_uuid(), $1, $2, 'common', 'weekly', NOW() - $3::interval)`,
				targetID, st, fmt.Sprintf("%d days", 20-i-c))
			if err != nil {
				log.Printf("stamp %s: %v", st, err)
			}
			stampCount++
		}
	}

	// Rare stamps (some stacked)
	rareStamps := []string{"rare:cross_country", "rare:cross_country", "rare:cross_country", "rare:explorer", "rare:penpal"}
	for i, st := range rareStamps {
		_, err := db.ExecContext(ctx,
			`INSERT INTO stamps (id, owner_id, stamp_type, rarity, earned_via, created_at)
			 VALUES (gen_random_uuid(), $1, $2, 'rare', 'transfer', NOW() - $3::interval)`,
			targetID, st, fmt.Sprintf("%d days", 10-i))
		if err != nil {
			log.Printf("stamp %s: %v", st, err)
		}
		stampCount++
	}

	fmt.Printf("Inserted %d stamps\n", stampCount)

	fmt.Println("\nDone! Seed data:")
	fmt.Println("  50 contacts")
	fmt.Println("  100 received (5 in-transit, 25 delivered, 70 read) — all properly encrypted")
	fmt.Println("  100 sent (8 in-transit, 32 delivered, 60 read)")
	fmt.Printf("  %d stamps\n", stampCount)
}
