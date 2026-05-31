# syntax=docker/dockerfile:1
# Stage 1: build the provision-user SPI jar
FROM maven:3.9-eclipse-temurin-17 AS spi-build
WORKDIR /build
COPY apps/keycloak-spi/pom.xml ./
COPY apps/keycloak-spi/src ./src
RUN mvn -B package -DskipTests

# Stage 2: bake the SPI into Keycloak and pre-compile for postgres
FROM quay.io/keycloak/keycloak:26.0.7
COPY --from=spi-build /build/target/*.jar /opt/keycloak/providers/
# kc.sh build runs once at image build time so `kc.sh start --optimized` skips
# the augmentation step at container start - faster boot in prod.
# --health-enabled is a BUILD-TIME option; setting KC_HEALTH_ENABLED at runtime
# under --optimized is rejected (the pod crashes). Bake it here.
RUN /opt/keycloak/bin/kc.sh build --db=postgres --health-enabled=true
EXPOSE 8080
ENTRYPOINT ["/opt/keycloak/bin/kc.sh"]
CMD ["start", "--optimized"]
