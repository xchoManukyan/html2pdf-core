// Command html2pdf converts HTML documents to PDF using only the Go
// standard library.
//
//	go run .                          # web UI on http://localhost:8080
//	go run . -addr :9000              # different port
//	go run . -in page.html -out page.pdf [-size Letter -landscape -margin 20 -numbers]
//	go run . -in site.zip                        # HTML + assets bundle
//	go run . -in page.html -remote -user-password secret -no-copy
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/xchoManukyan/html2pdf-core/internal/converter"
	"github.com/xchoManukyan/html2pdf-core/internal/fonts"
	"github.com/xchoManukyan/html2pdf-core/internal/pdf"
	"github.com/xchoManukyan/html2pdf-core/internal/server"
)

func main() {
	addr := flag.String("addr", ":8080", "address for the web server")
	in := flag.String("in", "", "convert this HTML file and exit (CLI mode)")
	out := flag.String("out", "", "output PDF path (default: input name with .pdf)")
	size := flag.String("size", "A4", "page size: A3, A4, A5, Letter, Legal")
	landscape := flag.Bool("landscape", false, "landscape orientation")
	margin := flag.Float64("margin", 15, "page margin in millimetres")
	numbers := flag.Bool("numbers", false, "print page numbers")
	cssLib := flag.String("css", "auto", "CSS frameworks: auto (detect from the page, several possible), none, or a list of tailwind, bootstrap5, bootstrap4 (e.g. bootstrap5,tailwind)")
	viewportW := flag.Float64("viewport", 0, "layout width in CSS pixels, scaled to the paper (e.g. 1280 for a desktop layout, 375 for mobile; 0 = paper width)")
	remote := flag.Bool("remote", false, "CLI: allow downloading http(s) images, stylesheets and fonts")
	allowRemote := flag.Bool("allow-remote", false, "server: let users enable remote resources (SSRF-protected)")
	userPW := flag.String("user-password", "", "password required to open the PDF")
	ownerPW := flag.String("owner-password", "", "password for full access to the PDF")
	noPrint := flag.Bool("no-print", false, "forbid printing (encrypts the PDF)")
	noCopy := flag.Bool("no-copy", false, "forbid copying text (encrypts the PDF)")
	root := flag.String("root", "", "CLI: folder the document may load files from (default: its own folder); use it when pages reference ../css or ../img")
	noModify := flag.Bool("no-modify", false, "forbid editing and annotating (encrypts the PDF)")
	encMethod := flag.String("encryption", "aes-256", "encryption of protected PDFs: aes-256, aes-128 or rc4-128 (only for very old readers)")
	pdfa := flag.Bool("pdfa", false, "write PDF/A-2b (archival): embedded fonts, XMP metadata, sRGB output intent; no encryption")
	forms := flag.Bool("forms", false, "make inputs, checkboxes, radio buttons, selects and textareas fillable PDF form fields")
	signCert := flag.String("sign-cert", "", "sign the PDF: PEM file with the signing certificate (and its chain)")
	signKey := flag.String("sign-key", "", "sign the PDF: PEM file with the private key (RSA or ECDSA, unencrypted; default: the -sign-cert file)")
	signReason := flag.String("sign-reason", "", "signature: reason shown by PDF readers")
	signLocation := flag.String("sign-location", "", "signature: location")
	signContact := flag.String("sign-contact", "", "signature: contact information")
	signName := flag.String("sign-name", "", "signature: signer name (default: the certificate's common name)")
	signToken := flag.String("sign-token", os.Getenv("HTML2PDF_SIGN_TOKEN"), "server: secret that signing requests must send (Authorization: Bearer TOKEN); required with -sign-cert (default: HTML2PDF_SIGN_TOKEN)")
	maxJobs := flag.Int("max-jobs", runtime.NumCPU(), "server: conversions that run at the same time; further requests wait")
	docName := flag.String("doc", "", "CLI with -in -: the document's path inside -root, for relative links (default document.html)")
	jsonOut := flag.Bool("json", false, "CLI: print a JSON report (file, size, time, CSS libraries, missing resources, or the error) for programs")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()

	var signer *pdf.Signature
	if *signCert != "" || *signKey != "" {
		var err error
		if signer, err = loadSigner(*signCert, *signKey); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		signer.Reason, signer.Location, signer.Contact, signer.Name = *signReason, *signLocation, *signContact, *signName
	}

	if *showVersion {
		fmt.Println(version)
		return
	}
	if *in != "" {
		req := converter.Request{
			PageSize: *size, Landscape: *landscape, MarginMM: *margin, PageNumbers: *numbers, ViewportWidth: *viewportW, CSSLib: *cssLib,
			AllowRemote: *remote, UserPassword: *userPW, OwnerPassword: *ownerPW, DenyPrint: *noPrint, DenyCopy: *noCopy, DenyModify: *noModify, Encryption: *encMethod, PDFA: *pdfa, Forms: *forms, Sign: signer,
		}
		// passwords can also come from the environment, so they don't show
		// up in the process list
		if req.UserPassword == "" {
			req.UserPassword = os.Getenv("HTML2PDF_USER_PASSWORD")
		}
		if req.OwnerPassword == "" {
			req.OwnerPassword = os.Getenv("HTML2PDF_OWNER_PASSWORD")
		}
		os.Exit(runCLI(*in, *out, *root, *docName, *jsonOut, req))
	}

	if signer != nil && *signToken == "" {
		// without a secret, anyone who can reach the server could have documents signed
		fmt.Fprintln(os.Stderr, "a server that signs needs -sign-token (or HTML2PDF_SIGN_TOKEN): signing requests must send it as \"Authorization: Bearer TOKEN\"")
		os.Exit(2)
	}

	// Index installed fonts in the background so the first request is fast.
	go fonts.System()

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(server.Config{AllowRemote: *allowRemote, Signer: signer, SignToken: *signToken, MaxJobs: *maxJobs}),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      120 * time.Second,
	}
	go func() {
		log.Printf("HTML to PDF converter listening on http://localhost%s", *addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(ctx)
	log.Println("server stopped")
}

// loadSigner reads the signing certificate and key (both may be in one file).
func loadSigner(certFile, keyFile string) (*pdf.Signature, error) {
	if certFile == "" {
		certFile = keyFile
	}
	if keyFile == "" {
		keyFile = certFile
	}
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		return nil, fmt.Errorf("signing certificate: %w", err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, fmt.Errorf("signing key: %w", err)
	}
	return pdf.ParseSigner(certPEM, keyPEM)
}
