package bootstrap

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"os"
	"path/filepath"
	"runtime"

	"github.com/gorilla/securecookie"
	"github.com/jessevdk/go-flags"
)

type Flags struct {
	ConfFile           string `long:"conf" default:"./conf.json" description:"Specify location of Factorio Server Control config file." env:"FSM_CONF"`
	FactorioDir        string `long:"dir" default:"./" description:"Specify location of Factorio directory." env:"FSM_DIR"`
	ServerIP           string `long:"host" default:"0.0.0.0" description:"Specify IP for webserver to listen on." env:"FSM_SERVER_IP"`
	FactorioIP         string `long:"game-bind-address" default:"0.0.0.0" description:"Specify IP for Factorio game server to listen on." env:"FSM_FACTORIO_IP"`
	FactorioPort       string `long:"port" default:"80" description:"Specify a port for the server." env:"FSM_PORT"`
	FactorioConfigFile string `long:"config" default:"config/config.ini" description:"Specify location of Factorio config.ini file." env:"FSM_FACTORIO_CONFIG_FILE"`
	FactorioMaxUpload  int64  `long:"max-upload" default:"20" description:"Maximum filesize for uploaded files in MB." env:"FSM_MAX_UPLOAD"`
	FactorioBinary     string `long:"bin" default:"bin/x64/factorio" description:"Location of Factorio Server binary file." env:"FSM_BINARY"`
	FactorioRconPort   int    `long:"rcon-port" default:"0" description:"Specify port for rcon admin console." env:"FSM_RCON_PORT"`
	GlibcCustom        string `long:"glibc-custom" default:"false" description:"By default false, if custom glibc is required set this to true and add glibc-loc and glibc-lib-loc parameters." env:"FSM_GLIBC_CUSTOM"`
	GlibcLocation      string `long:"glibc-loc" default:"/opt/glibc-2.18/lib/ld-2.18.so" description:"Location glibc ld.so file if needed (ex. /opt/glibc-2.18/lib/ld-2.18.so)." env:"FSM_GLIBC_LOCATION"`
	GlibcLibLoc        string `long:"glibc-lib-loc" default:"/opt/glibc-2.18/lib" description:"Location of glibc lib folder (ex. /opt/glibc-2.18/lib)." env:"FSM_GLIBC_LIB"`
	Autostart          string `long:"autostart" default:"false" description:"Start Factorio after Factorio Server Control initializes, default false [true/false]." env:"FSM_AUTOSTART"`
	ModPackDir         string `long:"mod-pack-dir" default:"./mod_packs" description:"Directory to store mod packs." env:"FSM_MODPACK_DIR"`
}

type Config struct {
	FactorioDir             string `json:"factorio_dir,omitempty"`
	FactorioSavesDir        string `json:"saves_dir,omitempty"`
	FactorioBaseModDir      string `json:"basemod_dir,omitempty"`
	FactorioModsDir         string `json:"mods_dir,omitempty"`
	FactorioModPackDir      string `json:"mod_pack_dir,omitempty"`
	FactorioConfigFile      string `json:"config_file,omitempty"`
	FactorioConfigDir       string `json:"config_directory,omitempty"`
	FactorioLog             string `json:"logfile,omitempty"`
	FactorioBinary          string `json:"factorio_binary,omitempty"`
	FactorioRconPort        int    `json:"rcon_port,omitempty"`
	FactorioRconPass        string `json:"rcon_pass,omitempty"`
	FactorioCredentialsFile string `json:"factorio_credentials_file,omitempty"`
	FactorioIP              string `json:"factorio_ip,omitempty"`
	FactorioAdminFile       string `json:"factorio_admin_file,omitempty"`
	ServerIP                string `json:"server_ip,omitempty"`
	ServerPort              string `json:"server_port,omitempty"`
	MaxUploadSize           int64  `json:"max_upload_size,omitempty"`
	DatabaseFile            string `json:"database_file,omitempty"`
	SQLiteDatabaseFile      string `json:"sq_lite_database_file,omitempty"`
	CookieEncryptionKey     string `json:"cookie_encryption_key,omitempty"`
	SettingsFile            string `json:"settings_file,omitempty"`
	LogFile                 string `json:"log_file,omitempty"`
	ConfFile                string `json:"-"`
	GlibcCustom             string `json:"-"`
	GlibcLocation           string `json:"-"`
	GlibcLibLoc             string `json:"-"`
	Autostart               string `json:"-"`
	ConsoleCacheSize        int    `json:"console_cache_size,omitempty"` // the amount of cached lines, inside the factorio output cache
	ConsoleLogFile          string `json:"console_log_file,omitempty"`
	ChatLogFile             string `json:"chat_log_file,omitempty"` // separate log file for chat (incl join/quit)
	Secure                  bool   `json:"secure"`                  // set to `false` to use this tool without SSL/TLS (Default: `true`)
}

// set Configs default values. JSON unmarshal will replace when it found something different
var instantiated = Config{
	ConsoleCacheSize: 25,
	Secure:           true,
}

func NewConfig(args []string) Config {
	var opts Flags
	parser := flags.NewParser(&opts, flags.Default|flags.IgnoreUnknown)

	_, err := parser.ParseArgs(args)
	if err != nil {
		switch flagsErr := err.(type) {
		case flags.ErrorType:
			if flagsErr == flags.ErrHelp {
				os.Exit(0)
			}
		default:
			os.Exit(1)
		}
	}

	instantiated.mapFlags(opts)
	instantiated.loadServerConfig()

	return instantiated
}

func GetConfig() Config {
	return instantiated
}

func (config *Config) updateConfigFile() {
	contents, err := os.ReadFile(config.ConfFile)
	failOnError(err, "Error reading config file")
	var conf Config
	err = json.Unmarshal(contents, &conf)
	failOnError(err, "Error decoding JSON config file")
	// The config contains the session signing key and localhost RCON password.
	// Restrict an existing native-deployment file before adding generated secrets.
	err = os.Chmod(config.ConfFile, 0600)
	failOnError(err, "Error restricting JSON config file permissions.")

	var resave bool

	// set cookie encryption key, if empty
	// also set it, if the base64 string is not valid
	_, base64Err := base64.StdEncoding.DecodeString(conf.CookieEncryptionKey)
	if conf.CookieEncryptionKey == "" || conf.CookieEncryptionKey == "topsecretkey" || base64Err != nil {
		log.Println("CookieEncryptionKey invalid or empty, create new random one")
		randomKey := securecookie.GenerateRandomKey(32)
		conf.CookieEncryptionKey = base64.StdEncoding.EncodeToString(randomKey)

		resave = true
	}

	if conf.FactorioRconPass == "" || conf.FactorioRconPass == "factorio_rcon" {
		// Replace an empty or legacy default password.
		conf.FactorioRconPass = GenerateRandomPassword()
		log.Println("RCON password was empty or default; generated a new value and stored it in the manager config")

		resave = true
	}

	if conf.DatabaseFile != "" {
		// Migrate leveldb to sqlite
		// set new db name
		// just rename the file from the old path
		dbFileDir := filepath.Dir(conf.DatabaseFile)
		conf.SQLiteDatabaseFile = filepath.Join(dbFileDir, "sqlite.db")

		MigrateLevelDBToSqlite(conf.DatabaseFile, conf.SQLiteDatabaseFile)

		// remove old db name
		conf.DatabaseFile = ""
		resave = true
	}

	if resave {
		contents, err = json.MarshalIndent(conf, "", "\t")
		failOnError(err, "Error encoding JSON config file.")
		contents = append(contents, '\n')
		err = os.WriteFile(config.ConfFile, contents, 0600)
		failOnError(err, "Error writing JSON config file.")
	}

}

// Loads server configuration files
// JSON config file contains default values,
// config file will overwrite any provided flags
func (config *Config) loadServerConfig() {
	// load and potentially update conf.json
	config.updateConfigFile()

	file, err := os.Open(config.ConfFile)
	failOnError(err, "Error loading config file.")
	defer file.Close()

	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	failOnError(err, "Error decoding JSON config file.")

	if !filepath.IsAbs(config.SettingsFile) {
		config.SettingsFile = filepath.Join(config.FactorioConfigDir, config.SettingsFile)
	}

	if config.FactorioBaseModDir == "" {
		config.FactorioBaseModDir = filepath.Join(config.FactorioDir, "data", "base")
	}

	if !filepath.IsAbs(config.FactorioAdminFile) {
		config.FactorioAdminFile = filepath.Join(config.FactorioConfigDir, config.FactorioAdminFile)
	}

	if config.FactorioRconPort == 0 {
		config.FactorioRconPort = randomPort()
		log.Println("Rcon port is empty, generated new one:", config.FactorioRconPort)
	}
}

// Returns random port to use for rcon connection
func randomPort() int {
	value, err := rand.Int(rand.Reader, big.NewInt(5000))
	if err != nil {
		panic(fmt.Errorf("generate random RCON port: %w", err))
	}
	return int(value.Int64()) + 40000
}

func (config *Config) mapFlags(flags Flags) {
	config.Autostart = flags.Autostart
	config.GlibcCustom = flags.GlibcCustom
	config.GlibcLocation = flags.GlibcLocation
	config.GlibcLibLoc = flags.GlibcLibLoc
	config.ConfFile = flags.ConfFile
	config.FactorioDir = flags.FactorioDir
	config.ServerIP = flags.ServerIP
	config.ServerPort = flags.FactorioPort
	config.FactorioIP = flags.FactorioIP
	config.FactorioSavesDir = filepath.Join(flags.FactorioDir, "saves")
	config.FactorioModsDir = filepath.Join(flags.FactorioDir, "mods")
	config.FactorioModPackDir = flags.ModPackDir
	config.FactorioConfigDir = filepath.Join(flags.FactorioDir, "config")
	config.FactorioConfigFile = filepath.Join(flags.FactorioDir, flags.FactorioConfigFile)
	config.FactorioCredentialsFile = "./factorio.auth"
	config.FactorioAdminFile = "server-adminlist.json"
	config.ConsoleLogFile = filepath.Join(flags.FactorioDir, "factorio-server-console.log")
	config.FactorioRconPort = flags.FactorioRconPort

	config.MaxUploadSize = flags.FactorioMaxUpload * 1024 * 1024
	log.Printf("Max upload: %d", config.MaxUploadSize)
	log.Printf("Conffile: %s", config.ConfFile)

	if filepath.IsAbs(flags.FactorioBinary) {
		config.FactorioBinary = flags.FactorioBinary
	} else {
		config.FactorioBinary = filepath.Join(flags.FactorioDir, flags.FactorioBinary)
	}

	if runtime.GOOS == "windows" {
		appdata := os.Getenv("APPDATA")
		config.FactorioLog = filepath.Join(appdata, "Factorio", "factorio-current.log")
	} else {
		config.FactorioLog = filepath.Join(config.FactorioDir, "factorio-current.log")
	}
}

func failOnError(err error, msg string) {
	if err != nil {
		log.Printf("%s: %s", msg, err)
		panic(fmt.Sprintf("%s: %s", msg, err))
	}
}
