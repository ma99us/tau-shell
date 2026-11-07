import java.util.Properties
import groovy.json.JsonSlurper

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

// Optional local signing: android/keystore.properties (gitignored) with
// storeFile, storePassword, keyAlias and keyPassword.
val keystoreFile = rootProject.file("keystore.properties")
val keystoreProps = Properties().apply {
    if (keystoreFile.exists()) keystoreFile.inputStream().use { load(it) }
}

val packageInfo = JsonSlurper().parse(rootProject.file("../frontend/package.json")) as Map<*, *>
val appVersion = (findProperty("appVersion") as String?) ?: packageInfo["version"] as String
require(appVersion.matches(Regex("""(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)([-+][0-9A-Za-z.-]+)?"""))) {
    "Invalid app version: $appVersion"
}
val versionParts = appVersion.substringBefore('-').substringBefore('+').split('.').map { it.toLong() }
require(versionParts[0] <= 210000 && versionParts[1] <= 99 && versionParts[2] <= 99) {
    "App version cannot be represented by the Android versionCode scheme: $appVersion"
}
val derivedVersionCode = versionParts[0] * 10000 + versionParts[1] * 100 + versionParts[2]
require(derivedVersionCode in 1..2100000000L) {
    "App version cannot be represented by the Android versionCode scheme: $appVersion"
}
val appVersionCode = (findProperty("appVersionCode") as String?)?.toLong() ?: derivedVersionCode
require(appVersionCode == derivedVersionCode) { "appVersionCode must match appVersion ($derivedVersionCode)" }

android {
    namespace = "com.taushell.phone"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.taushell.phone"
        // TLS 1.3 is only available from Android 10.
        minSdk = 29
        targetSdk = 35
        versionCode = appVersionCode.toInt()
        versionName = appVersion
    }

    signingConfigs {
        if (keystoreFile.exists()) {
            create("release") {
                storeFile = file(keystoreProps.getProperty("storeFile"))
                storePassword = keystoreProps.getProperty("storePassword")
                keyAlias = keystoreProps.getProperty("keyAlias")
                keyPassword = keystoreProps.getProperty("keyPassword")
            }
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"), "proguard-rules.pro")
            signingConfig = signingConfigs.findByName("release")
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    testOptions {
        unitTests.isReturnDefaultValues = true
    }
}

kotlin {
    compilerOptions {
        jvmTarget.set(org.jetbrains.kotlin.gradle.dsl.JvmTarget.JVM_17)
    }
}

dependencies {
    // QR scanning through Google Play services; the link can also be pasted.
    implementation("com.google.android.gms:play-services-code-scanner:16.1.0")

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}
