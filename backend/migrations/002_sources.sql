INSERT INTO data_sources(name, base_url, category) VALUES
('Smithsonian Global Volcanism Program','https://volcano.si.edu/','volcano-history'),
('USGS Earthquake Hazards Program','https://earthquake.usgs.gov/','earthquakes'),
('USGS Volcano Hazards Program','https://www.usgs.gov/programs/VHP','volcano'),
('PVMBG','https://magma.esdm.go.id/','indonesia-volcano'),
('BMKG','https://www.bmkg.go.id/','indonesia-weather-tsunami'),
('NOAA','https://www.noaa.gov/','weather-space'),
('NASA Earthdata','https://www.earthdata.nasa.gov/','satellite'),
('Copernicus Data Space','https://dataspace.copernicus.eu/','satellite'),
('VAAC','https://www.ssd.noaa.gov/VAAC/','ash'),
('GEBCO','https://www.gebco.net/','bathymetry'),
('IRIS/EarthScope','https://www.earthscope.org/','seismology')
ON CONFLICT DO NOTHING;
