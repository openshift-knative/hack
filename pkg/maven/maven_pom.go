package maven

import (
	"encoding/xml"
	"fmt"
	"log"
	"os"
	"regexp"
	"slices"
	"strings"
)

// Pom represents the root structure of the pom.xml file
type Pom struct {
	XMLName              xml.Name             `xml:"project"`
	GroupID              string               `xml:"groupId"`
	ArtifactID           string               `xml:"artifactId"`
	Version              string               `xml:"version"`
	Dependencies         Dependencies         `xml:"dependencies"`
	DependencyManagement DependencyManagement `xml:"dependencyManagement"`
	Properties           Properties           `xml:"properties"`
}

// DependencyManagement represents the <dependencyManagement> block in the pom.xml
type DependencyManagement struct {
	Dependencies Dependencies `xml:"dependencies"`
}

// Dependencies represents the <dependencies> block in the pom.xml
type Dependencies struct {
	Dependency []Dependency `xml:"dependency"`
}

// Dependency represents a single dependency block
type Dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
}

// Properties represents the <properties> block in the pom.xml file
type Properties struct {
	Entries map[string]string `xml:",any"`
}

func UpdatePomFile(metadata []Metadata, url string, path string) error {

	pomFileBytes, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", path, err)
	}
	p := &Pom{}
	if err := xml.Unmarshal(pomFileBytes, p); err != nil {
		return fmt.Errorf("failed to unmarshal pom file %q: %v", path, err)
	}

	metadataByGA := map[string]Metadata{}
	for _, m := range metadata {
		metadataByGA[depKey(m.GroupID, m.ArtifactID)] = m
	}

	propVariable := regexp.MustCompile("\\$\\{(\\S+)}")
	propValue := regexp.MustCompile("([0-9a-z._-]+)")

	versionRegex := regexp.MustCompile("([0-9]+)\\.?([0-9]+)?\\.?([0-9]+)?")

	pomFileStr := string(pomFileBytes)

	if !strings.Contains(pomFileStr, url) {

		repository := fmt.Sprintf(`
    <repository>
      <id>red-hat-ga</id>
      <url>%s</url>
      <releases>
        <enabled>true</enabled>
      </releases>
      <snapshots>
        <enabled>true</enabled>
      </snapshots>
    </repository>
`, url)

		repositoriesRegex := regexp.MustCompile("(<repositories>.*)([\\s\\S]*<repository>[\\s\\S]+</repositories>.*)")
		for _, match := range repositoriesRegex.FindAllStringSubmatch(pomFileStr, -1) {
			pomFileStr = strings.ReplaceAll(
				pomFileStr,
				match[0],
				fmt.Sprintf("%s%s%s", match[1], repository, match[2]),
			)
		}
	}

	for _, d := range p.DependencyManagement.Dependencies.Dependency {
		groupId := d.GroupID
		if d.GroupID == "io.quarkus" {
			groupId = "com.redhat.quarkus.platform"
		}
		m, ok := metadataByGA[depKey(groupId, d.ArtifactID)]
		if !ok {
			continue
		}
		version := d.Version
		propNameMatch := propVariable.FindStringSubmatch(d.Version)
		if len(propNameMatch) > 1 {
			version, ok = p.Properties.Entries[propNameMatch[1]]
			if !ok {
				return fmt.Errorf("failed to replace variable %q from properties %#v", d.Version, p.Properties.Entries)
			}
			if match := propValue.FindStringSubmatch(version); len(match) > 1 {
				version = match[1]
			}
		}

		log.Printf("Dependency %q:%q:%q\n%#v\n", d.GroupID, d.ArtifactID, version, m)

		versionPieces := versionRegex.FindStringSubmatch(version)
		if len(versionPieces) < 2 {
			log.Printf("Version %q is not parsable", version)
			continue
		}
		log.Printf("Version %q, pieces %#v", version, versionPieces)

		versions := make([]string, len(m.Versioning.Versions.Version))
		copy(versions, m.Versioning.Versions.Version)
		slices.Reverse(versions)

		for _, v := range versions {
			if v == version {
				break
			}

			vPieces := versionRegex.FindStringSubmatch(v)
			if len(vPieces) < 2 {
				log.Printf("Version %q in metadata is not parsable (%s:%s)", version, m.GroupID, m.ArtifactID)
				continue
			}
			log.Printf("Red Hat Version %q, pieces %#v", v, vPieces)

			if vPieces[1] != versionPieces[1] {
				// major version doesn't match
				log.Printf("Version %q doesn't match major version in %q", version, v)
				continue
			}

			if len(versionPieces) >= 3 && len(vPieces) >= 3 && vPieces[2] != versionPieces[2] {
				// minor version doesn't match
				log.Printf("Version %q doesn't match minor version in %q", version, v)
				continue
			}

			rg := fmt.Sprintf("(.*<groupId>\\s*)%s(\\s*</groupId>\\s*<artifactId>\\s*)%s(\\s*</artifactId>\\s*<version>\\s*)%s(\\s*</version>.*)", regexp.QuoteMeta(d.GroupID), regexp.QuoteMeta(d.ArtifactID), regexp.QuoteMeta(d.Version))
			log.Printf("Replacing %q:%q:%q with %q:%q:%q (regex %q)", d.GroupID, d.ArtifactID, version, m.GroupID, m.ArtifactID, v, rg)

			artifactRegex := regexp.MustCompile(rg)
			for _, match := range artifactRegex.FindAllStringSubmatch(pomFileStr, -1) {
				oldM := fmt.Sprintf("%s%s%s%s%s%s%s", match[1], d.GroupID, match[2], d.ArtifactID, match[3], d.Version, match[4])
				newM := fmt.Sprintf("%s%s%s%s%s%s%s", match[1], m.GroupID, match[2], m.ArtifactID, match[3], v, match[4])

				log.Printf("match:\n%s\n%s\n", oldM, newM)

				pomFileStr = strings.ReplaceAll(pomFileStr, oldM, newM)
			}
		}
	}

	if err := os.WriteFile(path, []byte(pomFileStr), 0666); err != nil {
		return fmt.Errorf("failed to update POM file %q: %w", path, err)
	}
	return nil
}

func depKey(groupId, artifactId string) string {
	return fmt.Sprintf("%s:%s", groupId, artifactId)
}

// UnmarshalXML is a custom unmarshaler for handling dynamic XML elements in <properties>
func (p *Properties) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	p.Entries = make(map[string]string)
	for {
		// Read the next token
		t, err := d.Token()
		if err != nil {
			return err
		}
		switch elem := t.(type) {
		case xml.StartElement:
			var value string
			// Decode the value of the element
			if err := d.DecodeElement(&value, &elem); err != nil {
				return err
			}
			// Use the element name as the key
			p.Entries[elem.Name.Local] = value
		case xml.EndElement:
			// Exit once we reach the end of the properties block
			if elem.Name.Local == "properties" {
				return nil
			}
		}
	}
}
