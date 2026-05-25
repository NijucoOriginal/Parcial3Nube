package handlers

import (
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// ─── Configuración ────────────────────────────────────────────────────────────

const (
	VBOXMANAGE        = `C:\Program Files\Oracle\VirtualBox\VBoxManage.exe`
	SSH_USER          = "nicolas"
	SSH_KEY_PATH      = `C:\Users\NICOLAS PEÑA RINCON\.ssh\id_compunube`
	MARIADB_TEMPLATE  = "mariadb"
	POSTGRES_TEMPLATE = "postgresSQl"
	MARIADB_DISK      = `C:\Users\NICOLAS PEÑA RINCON\VirtualBox VMs\mariadb\mariadb-disk1.vdi`
	POSTGRES_DISK     = `C:\Users\NICOLAS PEÑA RINCON\VirtualBox VMs\postgresSQl\postgresSQl-disk1.vdi`
	HOST_ONLY_NET     = "VirtualBox Host-Only Ethernet Adapter"
	HOST_ONLY_PREFIX  = "192.168.56."
	HOST_ONLY_BCAST   = "192.168.56.255"
)

// ─── Modelos ──────────────────────────────────────────────────────────────────

type Instance struct {
	VMName    string
	DBName    string
	DBUser    string
	DBPass    string
	Engine    string // "mariadb" o "postgresql"
	IP        string
	MAC       string
	State     string // "en ejecución" | "apagada"
	CreatedAt time.Time
	AccessCmd string
}

type LogEntry struct {
	Timestamp string
	Message   string
}

// ─── Estado en memoria ────────────────────────────────────────────────────────

var (
	mu        sync.Mutex
	instances []Instance
	logs      []LogEntry
)

func addLog(msg string) {
	entry := LogEntry{
		Timestamp: time.Now().Format("2006-01-02 15:04pm"),
		Message:   msg,
	}
	mu.Lock()
	logs = append(logs, entry)
	mu.Unlock()
	log.Println(msg)
}

// ─── Utilidades VBoxManage ────────────────────────────────────────────────────

func vbox(args ...string) (string, error) {
	out, err := exec.Command(VBOXMANAGE, args...).CombinedOutput()
	return string(out), err
}

// getMACAddress obtiene la MAC del adaptador 2 (Host-Only) de la VM
func getMACAddress(vmName string) (string, error) {
	out, err := vbox("showvminfo", vmName, "--machinereadable")
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "macaddress2=") {
			mac := strings.Trim(strings.Split(line, "=")[1], `"`)
			return formatMAC(mac), nil
		}
	}
	return "", fmt.Errorf("MAC no encontrada para %s", vmName)
}

// formatMAC convierte "080027AABBCC" a "08:00:27:aa:bb:cc"
func formatMAC(raw string) string {
	raw = strings.ToLower(raw)
	parts := make([]string, 0, 6)
	for i := 0; i < len(raw)-1; i += 2 {
		parts = append(parts, raw[i:i+2])
	}
	return strings.Join(parts, ":")
}

// getIPByMAC busca en la tabla ARP la IP correspondiente a una MAC
func getIPByMAC(mac string) (string, error) {
	// Ping broadcast para poblar la tabla ARP
	exec.Command("ping", "-n", "3", "-w", "500", HOST_ONLY_BCAST).Run()

	out, err := exec.Command("arp", "-a").Output()
	if err != nil {
		return "", err
	}

	mac = strings.ToLower(mac)
	// Windows muestra MACs con guiones: 08-00-27-aa-bb-cc
	macDash := strings.ReplaceAll(mac, ":", "-")

	for _, line := range strings.Split(string(out), "\n") {
		lineLower := strings.ToLower(line)
		if strings.Contains(lineLower, macDash) || strings.Contains(lineLower, mac) {
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				ip := fields[0]
				if strings.HasPrefix(ip, HOST_ONLY_PREFIX) {
					return ip, nil
				}
			}
		}
	}
	return "", fmt.Errorf("IP no encontrada para MAC %s", mac)
}

// waitForIP espera hasta que la VM tenga IP asignada por DHCP (máx 2 minutos)
func waitForIP(mac string) (string, error) {
	for i := 0; i < 24; i++ {
		ip, err := getIPByMAC(mac)
		if err == nil && ip != "" {
			return ip, nil
		}
		time.Sleep(5 * time.Second)
	}
	return "", fmt.Errorf("timeout esperando IP para MAC %s", mac)
}

// generatePassword genera una contraseña simple
func generatePassword() string {
	return fmt.Sprintf("pass%d", time.Now().UnixNano()%100000)
}

// ─── SSH ──────────────────────────────────────────────────────────────────────

func sshConnect(ip string) (*ssh.Client, error) {
	key, err := readPrivateKey(SSH_KEY_PATH)
	if err != nil {
		return nil, fmt.Errorf("error leyendo llave SSH: %v", err)
	}

	config := &ssh.ClientConfig{
		User:            SSH_USER,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(key)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	}

	return ssh.Dial("tcp", ip+":22", config)
}

func readPrivateKey(path string) (ssh.Signer, error) {
	out, err := exec.Command("cmd", "/C", "type", path).Output()
	if err != nil {
		return nil, err
	}
	return ssh.ParsePrivateKey(out)
}

func runSSH(client *ssh.Client, cmd string) (string, error) {
	sess, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer sess.Close()
	out, err := sess.CombinedOutput(cmd)
	return string(out), err
}

// ─── Configuración de hostname via SSH ───────────────────────────────────────

func setHostname(client *ssh.Client, hostname string) error {
	_, err := runSSH(client, fmt.Sprintf("sudo hostnamectl set-hostname %s", hostname))
	if err != nil {
		return err
	}
	_, err = runSSH(client, fmt.Sprintf("echo '%s' | sudo tee /etc/hostname", hostname))
	if err != nil {
		return err
	}
	_, err = runSSH(client, fmt.Sprintf(
		"sudo sed -i 's/127.0.1.1.*/127.0.1.1\t%s/' /etc/hosts || echo '127.0.1.1\t%s' | sudo tee -a /etc/hosts",
		hostname, hostname,
	))
	return err
}

// ─── Provisioning MariaDB ─────────────────────────────────────────────────────

func setupMariaDB(client *ssh.Client, dbName, dbUser, dbPass string) error {
	cmds := []string{
		fmt.Sprintf("sudo mysql -u root -e \"CREATE DATABASE IF NOT EXISTS %s;\"", dbName),
		fmt.Sprintf("sudo mysql -u root -e \"CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s';\"", dbUser, dbPass),
		fmt.Sprintf("sudo mysql -u root -e \"GRANT ALL PRIVILEGES ON %s.* TO '%s'@'%%';\"", dbName, dbUser),
		"sudo mysql -u root -e \"FLUSH PRIVILEGES;\"",
	}
	for _, cmd := range cmds {
		if _, err := runSSH(client, cmd); err != nil {
			return fmt.Errorf("error ejecutando '%s': %v", cmd, err)
		}
	}
	return nil
}

// ─── Provisioning PostgreSQL ──────────────────────────────────────────────────

func setupPostgreSQL(client *ssh.Client, dbName, dbUser, dbPass string) error {
	cmds := []string{
		fmt.Sprintf(`sudo -u postgres psql -c "CREATE USER %s WITH PASSWORD '%s';"`, dbUser, dbPass),
		fmt.Sprintf(`sudo -u postgres psql -c "CREATE DATABASE %s OWNER %s;"`, dbName, dbUser),
		fmt.Sprintf(`sudo -u postgres psql -c "GRANT ALL PRIVILEGES ON DATABASE %s TO %s;"`, dbName, dbUser),
	}
	for _, cmd := range cmds {
		if _, err := runSSH(client, cmd); err != nil {
			return fmt.Errorf("error ejecutando '%s': %v", cmd, err)
		}
	}
	return nil
}

// ─── Handlers HTTP ────────────────────────────────────────────────────────────

func IndexHandler(w http.ResponseWriter, r *http.Request) {
	tmpl := template.Must(template.ParseFiles("static/index.html"))
	mu.Lock()
	data := struct {
		Instances []Instance
		Logs      []LogEntry
	}{instances, logs}
	mu.Unlock()
	tmpl.Execute(w, data)
}

func ProvisionHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Método no permitido", http.StatusMethodNotAllowed)
		return
	}

	r.ParseMultipartForm(10 << 20)
	vmName := r.FormValue("vmName")
	dbUser := r.FormValue("dbUser")
	dbName := r.FormValue("dbName")
	engine := r.FormValue("engine")

	if vmName == "" || dbUser == "" || dbName == "" || engine == "" {
		http.Error(w, "Faltan campos requeridos", http.StatusBadRequest)
		return
	}

	var sqlContent string
	file, _, err := r.FormFile("sqlFile")
	if err == nil {
		defer file.Close()
		data, _ := io.ReadAll(file)
		sqlContent = string(data)
	}

	addLog(fmt.Sprintf("Solicitud de creación de la base de datos %s con usuario %s en %s", dbName, dbUser, engine))

	go func() {
		dbPass := generatePassword()

		var templateName, diskPath string
		if engine == "mariadb" {
			templateName = MARIADB_TEMPLATE
			diskPath = MARIADB_DISK
		} else {
			templateName = POSTGRES_TEMPLATE
			diskPath = POSTGRES_DISK
		}

		// 1. Crear nueva VM
		addLog(fmt.Sprintf("Creando VM %s a partir de plantilla %s", vmName, templateName))
		if _, err := vbox("createvm", "--name", vmName, "--ostype", "Debian_64", "--register"); err != nil {
			addLog(fmt.Sprintf("ERROR creando VM: %v", err))
			return
		}

		// 2. Configurar hardware
		vbox("modifyvm", vmName, "--memory", "1024", "--cpus", "1")
		vbox("modifyvm", vmName, "--nic1", "nat")
		vbox("modifyvm", vmName, "--nic2", "hostonly", "--hostonlyadapter2", HOST_ONLY_NET)

		// 3. Adjuntar controlador SATA
		vbox("storagectl", vmName, "--name", "SATA", "--add", "sata", "--controller", "IntelAHCI")

		// 4. Adjuntar disco multiconexión
		vbox("storageattach", vmName,
			"--storagectl", "SATA",
			"--port", "0",
			"--device", "0",
			"--type", "hdd",
			"--medium", diskPath,
			"--mtype", "multiattach",
		)

		// 5. Obtener MAC antes de iniciar
		mac, err := getMACAddress(vmName)
		if err != nil {
			addLog(fmt.Sprintf("ERROR obteniendo MAC: %v", err))
			return
		}
		addLog(fmt.Sprintf("MAC de %s: %s", vmName, mac))

		// 6. Iniciar VM
		addLog(fmt.Sprintf("Iniciando VM %s...", vmName))
		if _, err := vbox("startvm", vmName, "--type", "headless"); err != nil {
			addLog(fmt.Sprintf("ERROR iniciando VM: %v", err))
			return
		}
		addLog(fmt.Sprintf("Creación de la MV con %s para la ejecución de la base de datos %s", engine, dbName))

		// 7. Esperar IP por DHCP
		addLog(fmt.Sprintf("Esperando IP para %s...", vmName))
		ip, err := waitForIP(mac)
		if err != nil {
			addLog(fmt.Sprintf("ERROR obteniendo IP: %v", err))
			return
		}
		addLog(fmt.Sprintf("IP asignada a %s: %s", vmName, ip))

		// 8. Esperar a que SSH esté disponible
		time.Sleep(20 * time.Second)

		// 9. Conectar por SSH
		client, err := sshConnect(ip)
		if err != nil {
			addLog(fmt.Sprintf("ERROR conectando SSH a %s: %v", vmName, err))
			return
		}
		defer client.Close()

		// 10. Cambiar hostname
		if err := setHostname(client, vmName); err != nil {
			addLog(fmt.Sprintf("WARN cambiando hostname: %v", err))
		}

		// 11. Crear BD y usuario según motor
		if engine == "mariadb" {
			if err := setupMariaDB(client, dbName, dbUser, dbPass); err != nil {
				addLog(fmt.Sprintf("ERROR configurando MariaDB: %v", err))
				return
			}
			if sqlContent != "" {
				runSSH(client, fmt.Sprintf("sudo mysql -u root %s -e \"%s\"", dbName, sqlContent))
			}
		} else {
			if err := setupPostgreSQL(client, dbName, dbUser, dbPass); err != nil {
				addLog(fmt.Sprintf("ERROR configurando PostgreSQL: %v", err))
				return
			}
			if sqlContent != "" {
				runSSH(client, fmt.Sprintf(`sudo -u postgres psql -d %s -c "%s"`, dbName, sqlContent))
			}
		}

		// 12. Construir comando de acceso
		var accessCmd string
		if engine == "mariadb" {
			accessCmd = fmt.Sprintf("mariadb -u root -p%s", dbPass)
		} else {
			accessCmd = fmt.Sprintf("postgresql -u root -p%s", dbPass)
		}

		// 13. Guardar instancia
		mu.Lock()
		instances = append(instances, Instance{
			VMName:    vmName,
			DBName:    dbName,
			DBUser:    dbUser,
			DBPass:    dbPass,
			Engine:    engine,
			IP:        ip,
			MAC:       mac,
			State:     "en ejecución",
			CreatedAt: time.Now(),
			AccessCmd: accessCmd,
		})
		mu.Unlock()

		addLog(fmt.Sprintf("Base de datos %s lista en %s", dbName, ip))
	}()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func ListInstancesHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "[")
	for i, inst := range instances {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		fmt.Fprintf(w, `{"vmName":"%s","dbName":"%s","dbUser":"%s","dbPass":"%s","engine":"%s","ip":"%s","state":"%s","accessCmd":"%s"}`,
			inst.VMName, inst.DBName, inst.DBUser, inst.DBPass, inst.Engine, inst.IP, inst.State, inst.AccessCmd)
	}
	fmt.Fprintf(w, "]")
}

func DeleteInstanceHandler(w http.ResponseWriter, r *http.Request) {
	vmName := r.URL.Query().Get("vm")
	if vmName == "" {
		http.Error(w, "Falta parámetro vm", http.StatusBadRequest)
		return
	}

	addLog(fmt.Sprintf("Eliminando instancia %s...", vmName))

	go func() {
		vbox("controlvm", vmName, "poweroff")
		time.Sleep(3 * time.Second)
		vbox("unregistervm", vmName, "--delete")

		mu.Lock()
		for i, inst := range instances {
			if inst.VMName == vmName {
				instances = append(instances[:i], instances[i+1:]...)
				break
			}
		}
		mu.Unlock()
		addLog(fmt.Sprintf("Instancia %s eliminada", vmName))
	}()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func StopInstanceHandler(w http.ResponseWriter, r *http.Request) {
	vmName := r.URL.Query().Get("vm")
	if vmName == "" {
		http.Error(w, "Falta parámetro vm", http.StatusBadRequest)
		return
	}

	addLog(fmt.Sprintf("Apagando instancia %s...", vmName))

	go func() {
		vbox("controlvm", vmName, "acpipowerbutton")
		time.Sleep(5 * time.Second)

		mu.Lock()
		for i, inst := range instances {
			if inst.VMName == vmName {
				instances[i].State = "apagada"
				break
			}
		}
		mu.Unlock()
		addLog(fmt.Sprintf("Instancia %s apagada", vmName))
	}()

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func LogsHandler(w http.ResponseWriter, r *http.Request) {
	mu.Lock()
	defer mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "[")
	for i, l := range logs {
		if i > 0 {
			fmt.Fprintf(w, ",")
		}
		fmt.Fprintf(w, `{"timestamp":"%s","message":"%s"}`, l.Timestamp, l.Message)
	}
	fmt.Fprintf(w, "]")
}
