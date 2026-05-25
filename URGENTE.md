# Servicio Administrado de Bases de Datos (DBaaS) — Parcial 3

## Descripción General

Este proyecto implementa un prototipo funcional de un servicio administrado de bases de datos (Database as a Service - DBaaS) en un entorno local, simulando el funcionamiento de servicios reales como Amazon RDS o Google Cloud SQL.

La solución permite a los usuarios aprovisionar, configurar y gestionar bases de datos relacionales de forma automatizada a través de una aplicación web desarrollada en Go, utilizando infraestructura virtualizada con Oracle VirtualBox y máquinas virtuales con el sistema operativo GNU/Linux Debian (modo CLI).

### ¿Por qué Debian en modo CLI?

Se eligió Debian en modo CLI (sin interfaz gráfica) por las siguientes razones:

- Consume menos recursos de RAM y CPU, lo que permite correr múltiples instancias en el mismo equipo físico.
- Es más estable y predecible para entornos de servidor.
- Facilita la automatización mediante scripts y comandos SSH.

### Interfaces de red de cada VM

Cada máquina virtual fue configurada con dos interfaces de red:

**Adaptador 1 — NAT:** Permite que la VM tenga acceso a internet para instalar paquetes y actualizaciones. La VM obtiene una IP interna de VirtualBox (10.0.2.x) y se comunica con el exterior a través del equipo anfitrión.

**Adaptador 2 — Host-Only (Solo Anfitrión):** Crea una red privada exclusiva entre el equipo anfitrión (Windows) y las VMs. Se eligió este tipo de red porque:

- Permite la comunicación directa entre Windows y las VMs sin pasar por ningún router externo.
- Es completamente independiente de la red WiFi o Ethernet del anfitrión.
- Es la interfaz a través de la cual la aplicación Go se conecta por SSH a las VMs y por la que los usuarios acceden a las bases de datos con DBeaver.
- La red utilizada es `192.168.10.0/24` con el servidor DHCP de VirtualBox asignando IPs en el rango `192.168.10.10 - 192.168.10.100`.

### ¿Cómo funciona la aplicación?

```
Usuario
   |
   | (Formulario web)
   v
Aplicación Go (localhost:8080)
   |
   |-- 1. Crea nueva VM con VBoxManage
   |-- 2. Adjunta disco multiconexión (.vdi) de la plantilla
   |-- 3. Inicia la VM en modo headless
   |-- 4. Detecta la IP de la VM via tabla ARP (por MAC)
   |-- 5. Conecta por SSH con llave pública/privada
   |-- 6. Cambia el hostname de la VM
   |-- 7. Crea la base de datos y el usuario con privilegios
   |-- 8. Muestra los datos de conexión al usuario
   |
   v
VM con MariaDB o PostgreSQL corriendo
   |
   v
Usuario se conecta con DBeaver usando Host + Usuario + Contraseña
```

---

## Configuración

### 1. Configuración de las interfaces de red

#### Desde la línea de comandos (VBoxManage)

Primero crear la red Host-Only:

```cmd
"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" hostonlyif create
"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" hostonlyif ipconfig "VirtualBox Host-Only Ethernet Adapter" --ip 192.168.10.1 --netmask 255.255.255.0
"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" dhcpserver modify --ifname "VirtualBox Host-Only Ethernet Adapter" --ip 192.168.10.1 --netmask 255.255.255.0 --lowerip 192.168.10.10 --upperip 192.168.10.100 --enable
```

Configurar los adaptadores de red en cada VM (reemplazar `NOMBRE_VM` por el nombre de la VM):

```cmd
"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" modifyvm NOMBRE_VM --nic1 nat
"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" modifyvm NOMBRE_VM --nic2 hostonly --hostonlyadapter2 "VirtualBox Host-Only Ethernet Adapter"
```

> **Recomendación GUI:** También se puede hacer desde VirtualBox → Configuración → Red → Adaptador 1 (NAT) y Adaptador 2 (Solo anfitrión). Para el servidor DHCP ir a Archivo → Herramientas → Administrador de red de anfitrión → pestaña Servidor DHCP → Habilitar servidor.

---

### 2. Configuración de las interfaces de red dentro de cada VM

Editar el archivo `/etc/network/interfaces` en cada VM para que ambos adaptadores obtengan IP por DHCP automáticamente al arrancar:

```bash
sudo nano /etc/network/interfaces
```

El archivo debe contener:

```
auto lo
iface lo inet loopback

auto enp0s3
iface enp0s3 inet dhcp

auto enp0s8
iface enp0s8 inet dhcp
```

Guardar y reiniciar la red:

```bash
sudo systemctl restart networking
```

Verificar que `enp0s8` obtuvo IP en el rango `192.168.10.x`:

```bash
ip a
```

---

### 3. Cambiar el nombre del host de cada VM

Ejecutar dentro de cada VM:

```bash
# VM de MariaDB
sudo hostnamectl set-hostname mariadb
echo 'mariadb' | sudo tee /etc/hostname
sudo sed -i 's/127.0.1.1.*/127.0.1.1\tmariadb/' /etc/hosts

# VM de PostgreSQL
sudo hostnamectl set-hostname postgresSQl
echo 'postgresSQl' | sudo tee /etc/hostname
sudo sed -i 's/127.0.1.1.*/127.0.1.1\tpostgresSQl/' /etc/hosts
```

Verificar el cambio:

```bash
hostname
```

---

### 4. Generar y copiar las llaves SSH

**Generar las llaves en Windows** (desde PowerShell o CMD):

```cmd
ssh-keygen -t ed25519 -f "C:\Users\TU_USUARIO\.ssh\id_compunube"
```

**Copiar la llave pública a cada VM:**

```cmd
type "C:\Users\TU_USUARIO\.ssh\id_compunube.pub" | ssh USUARIO@192.168.10.10 "mkdir -p ~/.ssh && cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && chmod 700 ~/.ssh"

type "C:\Users\TU_USUARIO\.ssh\id_compunube.pub" | ssh USUARIO@192.168.10.11 "mkdir -p ~/.ssh && cat >> ~/.ssh/authorized_keys && chmod 600 ~/.ssh/authorized_keys && chmod 700 ~/.ssh"
```

**Configurar SSH en cada VM** para permitir acceso por llave pública:

```bash
sudo nano /etc/ssh/sshd_config
```

Asegurarse que estas líneas estén sin el `#`:

```
PubkeyAuthentication yes
AuthorizedKeysFile .ssh/authorized_keys
PermitRootLogin yes
```

Reiniciar SSH:

```bash
sudo systemctl restart ssh
```

**Verificar conexión desde Windows:**

```cmd
ssh -i "C:\Users\TU_USUARIO\.ssh\id_compunube" USUARIO@192.168.10.10
```

---

### 5. Configurar sudo sin contraseña

Para que la aplicación Go pueda ejecutar comandos administrativos por SSH sin interacción manual:

```bash
sudo visudo
```

Agregar al final del archivo:

```
USUARIO ALL=(ALL) NOPASSWD: ALL
```

---

### 6. Instalar MariaDB (VM plantilla-mariadb)

```bash
sudo apt update && sudo apt install -y mariadb-server
sudo systemctl enable mariadb
sudo systemctl start mariadb
```

Configurar acceso remoto en `/etc/mysql/mariadb.conf.d/50-server.cnf`:

```bash
sudo nano /etc/mysql/mariadb.conf.d/50-server.cnf
```

Cambiar:
```
bind-address = 0.0.0.0
```

Configurar usuario root con acceso remoto:

```bash
sudo mysql -u root
```

```sql
SET PASSWORD FOR 'root'@'localhost' = PASSWORD('tu_contraseña');
GRANT ALL PRIVILEGES ON *.* TO 'root'@'%' IDENTIFIED BY 'tu_contraseña' WITH GRANT OPTION;
FLUSH PRIVILEGES;
EXIT;
```

Reiniciar MariaDB:

```bash
sudo systemctl restart mariadb
```

---

### 7. Instalar PostgreSQL (VM plantilla-postgresql)

> Durante el desarrollo se utilizó **PostgreSQL 17**.

```bash
sudo apt update && sudo apt install -y postgresql
sudo systemctl enable postgresql
sudo systemctl start postgresql
```

Permitir conexiones remotas en `/etc/postgresql/17/main/postgresql.conf`:

```bash
sudo nano /etc/postgresql/17/main/postgresql.conf
```

Buscar y cambiar:
```
listen_addresses = '*'
```

Agregar al final de `/etc/postgresql/17/main/pg_hba.conf`:

```bash
echo "host    all    all    0.0.0.0/0    md5" | sudo tee -a /etc/postgresql/17/main/pg_hba.conf
```

Configurar usuarios:

```bash
sudo -u postgres psql
```

```sql
ALTER USER postgres WITH PASSWORD 'tu_contraseña';
CREATE USER root WITH SUPERUSER PASSWORD 'tu_contraseña';
\q
```

Reiniciar PostgreSQL:

```bash
sudo systemctl restart postgresql
```

---

### 8. Configurar disco multiconexión

Una vez configuradas las VMs plantilla y con todos los servicios funcionando, apagar cada VM y convertir su disco a modo multiconexión. Esto permite que múltiples VMs nuevas arranquen desde el mismo disco base sin modificarlo.

```cmd
"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" controlvm mariadb poweroff
"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" controlvm postgresSQl poweroff

"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" modifyhd "C:\Users\TU_USUARIO\VirtualBox VMs\mariadb\mariadb-disk1.vdi" --type multiattach
"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" modifyhd "C:\Users\TU_USUARIO\VirtualBox VMs\postgresSQl\postgresSQl-disk1.vdi" --type multiattach
```

Verificar que quedó en modo multiattach:

```cmd
"C:\Program Files\Oracle\VirtualBox\VBoxManage.exe" showhdinfo "C:\Users\TU_USUARIO\VirtualBox VMs\mariadb\mariadb-disk1.vdi" | findstr "Type"
```

Debe mostrar: `Type: Multiattach`

---

## Ejecutar la aplicación

```cmd
cd C:\ruta\al\proyecto\Parcial3Nube
go mod tidy
go run main.go
```

Abrir en el navegador: `http://localhost:8080`
