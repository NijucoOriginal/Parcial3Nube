-- Tabla de tipos de animales
CREATE TABLE tipos_animal (
    id INT AUTO_INCREMENT PRIMARY KEY,
    nombre VARCHAR(50) NOT NULL
);

-- Tabla de animales domésticos
CREATE TABLE animales_domesticos (
    id INT AUTO_INCREMENT PRIMARY KEY,
    nombre VARCHAR(100) NOT NULL,
    especie VARCHAR(100) NOT NULL,
    raza VARCHAR(100),
    edad INT,
    peso DECIMAL(5,2),
    color VARCHAR(50),
    vacunado BOOLEAN DEFAULT FALSE,
    fecha_registro DATE,
    id_tipo INT,
    FOREIGN KEY (id_tipo) REFERENCES tipos_animal(id)
);

-- Datos de prueba tipos
INSERT INTO tipos_animal (nombre) VALUES
('Perro'),
('Gato'),
('Conejo'),
('Hamster');

-- Datos de prueba animales
INSERT INTO animales_domesticos (nombre, especie, raza, edad, peso, color, vacunado, fecha_registro, id_tipo) VALUES
('Max', 'Canis lupus familiaris', 'Labrador', 3, 28.50, 'Amarillo', TRUE, '2024-01-10', 1),
('Luna', 'Felis catus', 'Siames', 2, 4.20, 'Blanco', TRUE, '2024-02-15', 2),
('Rocky', 'Canis lupus familiaris', 'Bulldog', 5, 22.00, 'Cafe', FALSE, '2024-03-20', 1),
('Michi', 'Felis catus', 'Persa', 1, 3.80, 'Gris', TRUE, '2024-04-05', 2),
('Toby', 'Oryctolagus cuniculus', 'Mini Lop', 2, 1.50, 'Blanco y negro', TRUE, '2024-05-12', 3),
('Sandy', 'Mesocricetus auratus', 'Sirio', 1, 0.15, 'Dorado', FALSE, '2024-06-18', 4);
