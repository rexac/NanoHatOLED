package nanohatoled

//display integration is based on github.com/mmalcek/nanohatoled project

import (
    "fmt"
    "image"
    "image/color"
    "image/draw"
    "os"

    "github.com/disintegration/imaging"
    "github.com/golang/freetype"
    "github.com/golang/freetype/truetype"
    "golang.org/x/image/font"
    "periph.io/x/periph/conn/gpio"
    "periph.io/x/periph/conn/gpio/gpioreg"
    "periph.io/x/periph/host"
    "golang.org/x/exp/io/i2c"
)

const (
    // SSD1306 basic commands
    ssd1306DisplayOn  = 0xAf
    ssd1306DisplayOff = 0xAe
    ssd1306ActivateScroll                   = 0x2F
    ssd1306DeactivateScroll                 = 0x2E
    ssd1306SetVerticalScrollArea            = 0xA3
    ssd1306RightHorizontalScroll            = 0x26
    ssd1306LeftHorizontalScroll             = 0x27
    ssd1306VerticalAndRightHorizontalScroll = 0x29
    ssd1306VerticalAndLeftHorizontalScroll  = 0x2A

    // Font paths (align with Python version)
    defaultFontPath    = "/etc/NanoHatOLED/DejaVuSansMono.ttf"
    defaultBoldFontPath = "/etc/NanoHatOLED/DejaVuSansMono-Bold.ttf"
    FixedDPI           = 72 // Match PIL default DPI for consistent font size
    
    // Dynamic threshold base value (adjust with font size)
    baseAntiAliasThresh = 90  // Base threshold (balance arc preservation + anti-aliasing)
    sizeThresholdSmall  = 12.0 // Small font size threshold
    sizeThresholdLarge  = 24.0 // Large font size threshold
)

// NanoOled - OLED controller with font support
type NanoOled struct {
    dev *i2c.Device

    w             int    // Screen width
    h             int    // Screen height
    buf           []byte // Pixel buffer
    rotation      int    // Screen rotation angle
    rotationState bool   // Rotation applied flag
    image         *image.NRGBA // Image buffer
    Btn           [3]gpio.PinIO // GPIO buttons

    // Font related fields
    normalFont  *truetype.Font // Regular monospace font
    boldFont    *truetype.Font // Bold monospace font
    currentFont *truetype.Font // Currently used font
    fontSize    float64        // Current font size
}

// init - Initialize SSD1306 OLED controller
func (nanoOled *NanoOled) init() (err error) {
    err = nanoOled.dev.Write([]byte{
        0xae,
        0x00 | 0x00, // Column offset low
        0x10 | 0x00, // Column offset high
        0xd5, 0x80,  // Set display clock divide ratio
        0xa8, uint8(nanoOled.h - 1), // Set multiplex ratio
        0xd3, 0x00, // Set display offset
        0x80 | 0,   // Set segment re-map
        0x8d, 0x14, // Enable charge pump
        0x20, 0x0,  // Set memory addressing mode
        0xA0 | 0x1, // Set COM output scan direction
        0xC8,       // Set COM pins hardware configuration
    })
    if err != nil {
        return
    }

    if nanoOled.h == 32 {
        err = nanoOled.dev.Write([]byte{
            0xda, 0x02, // Set COM pins configuration
            0x81, 0x8f, // Set contrast control
        })
    }

    if nanoOled.h == 64 {
        err = nanoOled.dev.Write([]byte{
            0xda, 0x12, // Set COM pins configuration
            0x81, 0x7f, // Set contrast control
        })
    }

    err = nanoOled.dev.Write([]byte{
        0x9d, 0xf1, // Set pre-charge period
        0xdb, 0x40, // Set VCOMH deselect level
        0xa4,       // Disable entire display on
        0xa6,       // Set normal display
        0x2e,       // Deactivate scroll
        0xaf,       // Turn on display
    })
    return
}

// loadFontFile - Load truetype font from file path
func loadFontFile(path string) (*truetype.Font, error) {
    fontBytes, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("load font failed: %w", err)
    }
    return truetype.Parse(fontBytes)
}

// Open - Initialize OLED and buttons, load fonts
func Open() (*NanoOled, error) {
    dev, err := i2c.Open(&i2c.Devfs{Dev: "/dev/i2c-0"}, 0x3C)
    if err != nil {
        return nil, fmt.Errorf("open I2C failed: %w", err)
    }

    buf := make([]byte, 128*(64/8)+1)
    buf[0] = 0x40 // Data command prefix
    oled := &NanoOled{
        dev:      dev,
        w:        128,
        h:        64,
        buf:      buf,
        fontSize: 14, // Default font size
    }

    // Load regular and bold fonts
    oled.normalFont, err = loadFontFile(defaultFontPath)
    if err != nil {
        return nil, fmt.Errorf("load regular font failed: %w", err)
    }
    oled.boldFont, err = loadFontFile(defaultBoldFontPath)
    if err != nil {
        return nil, fmt.Errorf("load bold font failed: %w", err)
    }
    oled.currentFont = oled.normalFont

    // Initialize OLED display
    if err := oled.init(); err != nil {
        return nil, fmt.Errorf("OLED init failed: %w", err)
    }

    // Initialize host peripherals
    if _, err := host.Init(); err != nil {
        fmt.Printf("Host init warning: %v\n", err)
    }

    // Initialize GPIO buttons
    btnPins := []string{"0", "2", "3"}
    for i, pinName := range btnPins {
        pin := gpioreg.ByName(pinName)
        if pin == nil {
            return nil, fmt.Errorf("GPIO%s not found", pinName)
        }
        if err := pin.In(gpio.PullNoChange, gpio.RisingEdge); err != nil {
            return nil, fmt.Errorf("GPIO%s init failed: %w", pinName, err)
        }
        oled.Btn[i] = pin
    }

    return oled, nil
}

// OpenBtn - Empty implementation for Python compatibility
func OpenBtn() {}

// On - Turn on OLED display
func (nanoOled *NanoOled) On() error {
    return nanoOled.dev.Write([]byte{ssd1306DisplayOn})
}

// Off - Turn off OLED display
func (nanoOled *NanoOled) Off() error {
    return nanoOled.dev.Write([]byte{ssd1306DisplayOff})
}

// Close - Close I2C connection
func (nanoOled *NanoOled) Close() error {
    return nanoOled.dev.Close()
}

// New - Create new image buffer with specified rotation
func (nanoOled *NanoOled) New(rotation int) {
    nanoOled.rotation = rotation
    nanoOled.rotationState = false
    nanoOled.Clear()
    if rotation == 90 || rotation == 270 {
        nanoOled.image = image.NewNRGBA(image.Rect(0, 0, 64, 128))
    } else {
        nanoOled.image = image.NewNRGBA(image.Rect(0, 0, 128, 64))
    }
}

// getDynamicThreshold - Get dynamic binarization threshold based on font size
func (nanoOled *NanoOled) getDynamicThreshold() uint16 {
    var thresh uint16
    switch {
    case nanoOled.fontSize <= sizeThresholdSmall:
        thresh = baseAntiAliasThresh - 10 // Strict threshold for small fonts (preserve arcs)
    case nanoOled.fontSize >= sizeThresholdLarge:
        thresh = baseAntiAliasThresh + 10 // Loose threshold for large fonts (anti-aliasing)
    default:
        thresh = baseAntiAliasThresh // Base threshold for medium fonts
    }
    return thresh * 256 // Convert to RGBA range (0-65535)
}

// Image - Load and draw image to OLED buffer
func (nanoOled *NanoOled) Image(imagePath string) error {
    img, err := imaging.Open(imagePath)
    if err != nil {
        return fmt.Errorf("open image failed: %w", err)
    }

    // Resize image to fit screen
    img = imaging.Fit(img, 128, 64, imaging.NearestNeighbor)

    // Convert to grayscale and binarize with dynamic threshold
    grayImg := imaging.Grayscale(img)
    binaryImg := image.NewNRGBA(grayImg.Bounds())
    draw.Draw(binaryImg, binaryImg.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)

    // Get dynamic threshold (adapt to arc preservation for different fonts)
    threshold := nanoOled.getDynamicThreshold()
    
    for y := 0; y < grayImg.Bounds().Dy(); y++ {
        for x := 0; x < grayImg.Bounds().Dx(); x++ {
            r, g, b, _ := grayImg.At(x, y).RGBA()
            // Normalized grayscale calculation (RGBA range 0-65535)
            gray := uint16((r + g + b) / 3)
            // Set white only if grayscale exceeds dynamic threshold (preserve arcs)
            if gray > threshold {
                binaryImg.Set(x, y, color.White)
            }
        }
    }

    nanoOled.image = binaryImg
    return nil
}

// Send - Flush image buffer to OLED screen
func (nanoOled *NanoOled) Send() error {
    if nanoOled.rotationState == false {
        switch nanoOled.rotation {
        case 90:
            nanoOled.image = imaging.Rotate90(nanoOled.image)
            nanoOled.rotationState = true
        case 180:
            nanoOled.image = imaging.Rotate180(nanoOled.image)
            nanoOled.rotationState = true
        case 270:
            nanoOled.image = imaging.Rotate270(nanoOled.image)
            nanoOled.rotationState = true
        default:
        }
    }

    imgW := nanoOled.image.Bounds().Dx()
    imgH := nanoOled.image.Bounds().Dy()

    endX := 0 + imgW
    endY := 0 + imgH

    if endX >= nanoOled.w {
        endX = nanoOled.w
    }
    if endY >= nanoOled.h {
        endY = nanoOled.h
    }

    // Get dynamic threshold (adapt to font rendering)
    threshold := nanoOled.getDynamicThreshold()
    
    var imgI, imgY int
    for i := 0; i < endX; i++ {
        imgY = 0
        for j := 0; j < endY; j++ {
            r, g, b, _ := nanoOled.image.At(imgI, imgY).RGBA()
            var v byte
            // Precise grayscale judgment (preserve high-brightness pixels only)
            gray := (r + g + b) / 3
            if gray > uint32(threshold) {
                v = 0x1
            } else {
                v = 0x0
            }
            if err := nanoOled.setPixel(i, j, v); err != nil {
                return err
            }
            imgY++
        }
        imgI++
    }
    return nanoOled.draw()
}

// Clear - Clear OLED buffer and screen
func (nanoOled *NanoOled) Clear() error {
    if nanoOled.rotation == 90 || nanoOled.rotation == 270 {
        nanoOled.image = image.NewNRGBA(image.Rect(0, 0, 64, 128))
    } else {
        nanoOled.image = image.NewNRGBA(image.Rect(0, 0, 128, 64))
    }
    for i := 1; i < len(nanoOled.buf); i++ {
        nanoOled.buf[i] = 0
    }
    return nanoOled.draw()
}

// setPixel - Set single pixel value in buffer
func (nanoOled *NanoOled) setPixel(x, y int, v byte) error {
    if x >= nanoOled.w || y >= nanoOled.h {
        return fmt.Errorf("coordinate(%d,%d) out of screen(%dx%d)", x, y, nanoOled.w, nanoOled.h)
    }
    if v > 1 {
        return fmt.Errorf("pixel value must be 0 or 1, current:%d", v)
    }
    i := 1 + x + (y/8)*nanoOled.w
    if v == 0 {
        nanoOled.buf[i] &= ^(1 << uint((y & 7)))
    } else {
        nanoOled.buf[i] |= 1 << uint((y & 7))
    }
    return nil
}

// draw - Send pixel buffer to OLED via I2C
func (nanoOled *NanoOled) draw() error {
    if err := nanoOled.dev.Write([]byte{
        0xa4,     // Normal display mode
        0x40 | 0, // Set start line
        0x21, 0, uint8(nanoOled.w), // Set column range
        0x22, 0, 7,                 // Set page range
    }); err != nil {
        return fmt.Errorf("draw init failed: %w", err)
    }
    return nanoOled.dev.Write(nanoOled.buf)
}

// SetFontSize - Set current font size (max 32 to avoid screen overflow)
func (nanoOled *NanoOled) SetFontSize(size float64) {
    if size > 32 {
        size = 32
    }
    nanoOled.fontSize = size
}

// SetBold - Toggle bold font mode
func (nanoOled *NanoOled) SetBold(isBold bool) {
    if isBold {
        nanoOled.currentFont = nanoOled.boldFont
    } else {
        nanoOled.currentFont = nanoOled.normalFont
    }
}

// Text - Draw text to image buffer (anti-aliasing + equal vertical width + arc preservation)
// Fix: Remove incorrect Metrics call, use compatible baseline calibration
func (nanoOled *NanoOled) Text(x int, y int, text string, textColor bool) {
    // Boundary check: prevent text from exceeding screen
    if x < 0 {
        x = 0
    }
    if y < 0 {
        y = 0
    }
    // Calculate max Y to avoid overflow
    maxY := nanoOled.h - int(nanoOled.fontSize) - 2
    if y > maxY {
        y = maxY
    }

    // Set text color
    c := color.White
    if !textColor {
        c = color.Black
    }

    // Initialize freetype context with optimized anti-alias settings
    cr := freetype.NewContext()
    cr.SetDPI(FixedDPI)
    cr.SetFont(nanoOled.currentFont)
    cr.SetFontSize(nanoOled.fontSize)
    cr.SetHinting(font.HintingFull) // Full hinting for clear font edges
    cr.SetSrc(image.NewUniform(c))
    cr.SetDst(nanoOled.image)
    cr.SetClip(nanoOled.image.Bounds())

    // Calculate baseline by font size (offset adapts to vertical spacing)
    var baselineOffset float64
    switch {
    case nanoOled.fontSize <= 12:
        baselineOffset = 2.0 // Small font offset
    case nanoOled.fontSize <= 24:
        baselineOffset = 3.0 // Medium font offset
    default:
        baselineOffset = 4.0 // Large font offset
    }
    // Calculate baseline for vertical centering (equal top/bottom spacing)
    baseY := y + int(cr.PointToFixed(nanoOled.fontSize)>>6) - int(baselineOffset)
    pt := freetype.Pt(x, baseY)

    // Draw text with anti-aliasing
    if _, err := cr.DrawString(text, pt); err != nil {
        fmt.Printf("draw text failed: %v\n", err)
    }
}

// Pixel - Draw single pixel to image buffer
func (nanoOled *NanoOled) Pixel(x int, y int, pixColor bool) {
    // Boundary check
    if x < 0 || x >= nanoOled.w || y < 0 || y >= nanoOled.h {
        return
    }
    rColor := color.White
    if !pixColor {
        rColor = color.Black
    }
    nanoOled.image.Set(x, y, rColor)
}

// LineH - Draw horizontal line (optimized for equal vertical width)
func (nanoOled *NanoOled) LineH(x int, y int, length int, lineColor bool) {
    // Boundary check
    if x < 0 {
        x = 0
    }
    if y < 0 || y >= nanoOled.h {
        return
    }
    endX := x + length
    if endX >= nanoOled.w {
        endX = nanoOled.w - 1
    }

    lColor := color.White
    if !lineColor {
        lColor = color.Black
    }
    px := x
    for px <= endX {
        nanoOled.image.Set(px, y, lColor)
        px++
    }
}

// LineV - Draw vertical line (optimized for equal vertical width)
func (nanoOled *NanoOled) LineV(x int, y int, length int, lineColor bool) {
    // Boundary check
    if x < 0 || x >= nanoOled.w {
        return
    }
    if y < 0 {
        y = 0
    }
    endY := y + length
    if endY >= nanoOled.h {
        endY = nanoOled.h - 1
    }

    lColor := color.White
    if !lineColor {
        lColor = color.Black
    }
    py := y
    for py <= endY {
        nanoOled.image.Set(x, py, lColor)
        py++
    }
}

// Rect - Draw filled rectangle (optimized for equal vertical width)
func (nanoOled *NanoOled) Rect(MinX int, MinY int, MaxX int, MaxY int, rectColor bool) {
    // Boundary check: ensure rectangle is within screen
    if MinX < 0 {
        MinX = 0
    }
    if MinY < 0 {
        MinY = 0
    }
    if MaxX >= nanoOled.w {
        MaxX = nanoOled.w - 1
    }
    if MaxY >= nanoOled.h {
        MaxY = nanoOled.h - 1
    }
    if MinX > MaxX || MinY > MaxY {
        return
    }

    rColor := color.White
    if !rectColor {
        rColor = color.Black
    }
    py := MinY
    for py <= MaxY {
        px := MinX
        for px <= MaxX {
            nanoOled.image.Set(px, py, rColor)
            px++
        }
        py++
    }
}