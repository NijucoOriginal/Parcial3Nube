-- Tabla de departamentos
CREATE TABLE departamentos (
    id INT AUTO_INCREMENT PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    ubicacion VARCHAR(100)
);

-- Tabla de empleados
CREATE TABLE empleados (
    id INT AUTO_INCREMENT PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    apellido VARCHAR(100) NOT NULL,
    correo VARCHAR(150) UNIQUE,
    cargo VARCHAR(100),
    salario DECIMAL(10,2),
    fecha_ingreso DATE,
    id_departamento INT,
    FOREIGN KEY (id_departamento) REFERENCES departamentos(id)
);

-- Datos de prueba departamentos
INSERT INTO departamentos (nombre, ubicacion) VALUES
('Sistemas', 'Piso 3'),
('Recursos Humanos', 'Piso 1'),
('Contabilidad', 'Piso 2');

-- Datos de prueba empleados
INSERT INTO empleados (nombre, apellido, correo, cargo, salario, fecha_ingreso, id_departamento) VALUES
('Nicolas', 'Peña', 'nicolas@empresa.com', 'Desarrollador', 3500000.00, '2024-01-15', 1),
('Laura', 'Gomez', 'laura@empresa.com', 'Analista', 3000000.00, '2024-03-01', 1),
('Carlos', 'Ramirez', 'carlos@empresa.com', 'Gerente RRHH', 4500000.00, '2023-06-10', 2),
('Maria', 'Lopez', 'maria@empresa.com', 'Contadora', 3200000.00, '2023-09-20', 3);
